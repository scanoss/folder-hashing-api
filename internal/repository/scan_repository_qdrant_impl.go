// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2024 SCANOSS.COM
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 2 of the License, or
 * (at your option) any later version.
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/qdrant/go-client/qdrant"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

const (
	// VectorDim is the dimensionality of hash vectors stored in Qdrant.
	VectorDim = 64 // Single 64-bit hash per collection
)

// QdrantConfig contains configuration for connecting to Qdrant.
type QdrantConfig struct {
	Host string
	Port int
}

// ScanRepositoryQdrantImpl implements the ScanRepository interface using Qdrant.
type ScanRepositoryQdrantImpl struct {
	client *qdrant.Client
	config QdrantConfig
}

// NewScanRepositoryQdrantImpl creates a new Qdrant-based scan repository.
func NewScanRepositoryQdrantImpl(config QdrantConfig) (ScanRepository, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, errors.New("failed to connect to Qdrant: " + err.Error())
	}

	return &ScanRepositoryQdrantImpl{
		client: client,
		config: config,
	}, nil
}

// SearchByHashes performs a multi-stage prefetch search using directory, name, and content hashes.
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, rankThreshold int) ([]entities.ComponentGroup, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Info("Starting optimized multi-stage search")

	// Determine target collection based on primary language
	collectionName := entities.GetCollectionNameFromLanguageExtensions(langExt)
	s.Infof("Using collection: %s for language extensions: %v", collectionName, langExt)

	// Check if collection exists
	exists, err := r.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, errors.New("failed to check collection: " + err.Error())
	}
	if !exists {
		s.Warn("Collection %s does not exist, falling back to misc_collection", collectionName)
		collectionName = "misc_collection"

		// Check if misc collection exists
		exists, err = r.client.CollectionExists(ctx, collectionName)
		if err != nil || !exists {
			return nil, errors.New("collection " + collectionName + " does not exist")
		}
	}

	// Convert hashes to vectors
	nameVector, err := r.hexSimhashToVector(nameHash, VectorDim)
	if err != nil {
		return nil, errors.New("failed to convert name hash to vector: " + err.Error())
	}
	contentVector, err := r.hexSimhashToVector(contentHash, VectorDim)
	if err != nil {
		return nil, errors.New("failed to convert content hash to vector: " + err.Error())
	}
	dirsVector, err := r.hexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, errors.New("failed to convert dir hash to vector: " + err.Error())
	}

	// Execute optimized query
	searchResp, err := r.executeOptimizedQuery(ctx, collectionName, nameVector, dirsVector, contentVector, rankThreshold)
	if err != nil {
		return nil, fmt.Errorf("error performing optimized search in %s: %w", collectionName, err)
	}

	return r.processSearchResults(searchResp)
}

// createRangeRankCondition creates a condition for rank range matching (rank <= maxRank).
func (r *ScanRepositoryQdrantImpl) createRangeRankCondition(maxRank int) []*qdrant.Condition {
	return []*qdrant.Condition{
		{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "rank",
					Range: &qdrant.Range{
						Lte: qdrant.PtrOf(float64(maxRank)),
					},
				},
			},
		},
	}
}

// createExactRankCondition creates a condition for exact rank matching.
func (r *ScanRepositoryQdrantImpl) createExactRankCondition(rank int) []*qdrant.Condition {
	return []*qdrant.Condition{
		{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "rank",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Integer{
							Integer: int64(rank),
						},
					},
				},
			},
		},
	}
}

// Then use nested prefetches to apply progressive filtering.
//
//nolint:funlen // Multi-stage query complexity is inherent to the search algorithm
func (r *ScanRepositoryQdrantImpl) executeOptimizedQuery(ctx context.Context, collectionName string, nameVector, dirsVector, contentVector []float32, rankThreshold int) ([]*qdrant.ScoredPoint, error) {
	// Create rank filters
	rankThresholdConditions := r.createRangeRankCondition(rankThreshold)
	rankThresholdFilter := &qdrant.Filter{Must: rankThresholdConditions}

	exactRank1Conditions := r.createExactRankCondition(1)
	exactRank1Filter := &qdrant.Filter{Must: exactRank1Conditions}

	// Build the query with proper prefetch structure
	// The prefetch queries run FIRST and their results are used by the main query
	hybridQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Prefetch: []*qdrant.PrefetchQuery{
			// Branch 1: Start with broad name search
			{
				Query:  qdrant.NewQuery(nameVector...),
				Using:  qdrant.PtrOf("names"),
				Filter: rankThresholdFilter,
				Params: &qdrant.SearchParams{
					Exact: qdrant.PtrOf(true),
				},
				Limit:          qdrant.PtrOf(uint64(40000)), // Very broad initial search
				ScoreThreshold: qdrant.PtrOf(float32(30)),   // limit to garbage results
			},
			// Branch 2: Ensure rank 1 components are included
			{
				Query:  qdrant.NewQuery(nameVector...),
				Using:  qdrant.PtrOf("names"),
				Filter: exactRank1Filter,
				Params: &qdrant.SearchParams{
					Exact: qdrant.PtrOf(true),
				},
				Limit:          qdrant.PtrOf(uint64(40000)), // Dedicated search for popular components
				ScoreThreshold: qdrant.PtrOf(float32(30)),   // limit to garbage results
			},
		},
		// After prefetch collects candidates by name, apply content filtering
		Query:  qdrant.NewQuery(contentVector...),
		Using:  qdrant.PtrOf("contents"),
		Filter: rankThresholdFilter,
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit:       qdrant.PtrOf(uint64(60000)), // Filter down by content similarity
		WithPayload: qdrant.NewWithPayload(false),
		WithVectors: qdrant.NewWithVectors(false),
	}

	// Execute the first stage
	stage1Results, err := r.client.Query(ctx, hybridQuery)
	if err != nil {
		return nil, fmt.Errorf("stage 1 search failed: %w", err)
	}

	fmt.Printf("Stage 1 query returned %d results\n", len(stage1Results))

	// If we have few results, fetch payload and return
	if len(stage1Results) <= 20 {
		fmt.Printf("Returning early with %d results (<=20)\n", len(stage1Results))
		return r.fetchResultsWithPayload(ctx, collectionName, stage1Results)
	}

	// Stage 2: Apply dirs filtering on the results from stage 1
	stage1IDs := make([]uint64, len(stage1Results))
	for i, point := range stage1Results {
		stage1IDs[i] = point.Id.GetNum()
	}

	stage2Filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(stage1IDs),
					},
				},
			},
		},
	}

	// Apply dirs filtering
	stage2Query := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(dirsVector...),
		Using:          qdrant.PtrOf("dirs"),
		Filter:         stage2Filter,
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit:       qdrant.PtrOf(uint64(40000)), // Reduce to 20000
		WithPayload: qdrant.NewWithPayload(false),
		WithVectors: qdrant.NewWithVectors(false),
	}

	stage2Results, err := r.client.Query(ctx, stage2Query)
	if err != nil {
		return nil, fmt.Errorf("stage 2 search failed: %w", err)
	}

	fmt.Printf("Stage 2 query (dirs filtering) returned %d results\n", len(stage2Results))

	// If we have 20 or fewer results, return them
	if len(stage2Results) <= 20 {
		fmt.Printf("Returning with %d results after stage 2 (<=20)\n", len(stage2Results))
		return r.fetchResultsWithPayload(ctx, collectionName, stage2Results)
	}

	// Stage 3: Final refinement using nested prefetch for the best results
	stage2IDs := make([]uint64, 0, len(stage2Results))
	for _, point := range stage2Results {
		if point.Score > (stage2Results[0].Score*1.1)+5 {
			break
		}
		stage2IDs = append(stage2IDs, point.Id.GetNum())
	}

	finalFilter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(stage2IDs),
					},
				},
			},
		},
	}

	finalFilterPreferred := &qdrant.Filter{
		Must: append(
			[]*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_HasId{
						HasId: &qdrant.HasIdCondition{
							HasId: r.convertIDsToPointIDs(stage2IDs), // Nota: debería ser stage3IDs, no stage2IDs
						},
					},
				},
			},
			exactRank1Conditions...,
		),
	}

	// Final query with nested prefetch for optimal ranking
	dirsThreshold := stage2Results[0].Score*1.1 + 5
	finalQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Prefetch: []*qdrant.PrefetchQuery{
			// Re-rank by names within our candidate set
			{
				Query:  qdrant.NewQuery(dirsVector...),
				Using:  qdrant.PtrOf("dirs"),
				Filter: finalFilter,
				Params: &qdrant.SearchParams{
					Exact: qdrant.PtrOf(true),
				},
				Limit:          qdrant.PtrOf(uint64(1000)),
				ScoreThreshold: qdrant.PtrOf(dirsThreshold),
				Prefetch: []*qdrant.PrefetchQuery{
					// Then by dirs
					{
						Query:  qdrant.NewQuery(nameVector...),
						Using:  qdrant.PtrOf("names"),
						Filter: finalFilter,
						Params: &qdrant.SearchParams{
							Exact: qdrant.PtrOf(true),
						},
						Limit: qdrant.PtrOf(uint64(1000)),
					},
					{
						Query:  qdrant.NewQuery(nameVector...),
						Using:  qdrant.PtrOf("names"),
						Filter: finalFilterPreferred,
						Params: &qdrant.SearchParams{
							Exact: qdrant.PtrOf(true),
						},
						Limit: qdrant.PtrOf(uint64(1000)),
					},
				},
			},
		},
		// Final ranking by content
		Query:       qdrant.NewQuery(contentVector...),
		Using:       qdrant.PtrOf("contents"),
		Filter:      finalFilter,
		WithPayload: qdrant.NewWithPayload(true),
		WithVectors: qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(100)), // Final results
	}

	return r.client.Query(ctx, finalQuery)
}

// convertIDsToPointIDs converts uint64 IDs to PointId format.
func (r *ScanRepositoryQdrantImpl) convertIDsToPointIDs(ids []uint64) []*qdrant.PointId {
	pointIDs := make([]*qdrant.PointId, len(ids))
	for i, id := range ids {
		pointIDs[i] = &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Num{
				Num: id,
			},
		}
	}
	return pointIDs
}

// fetchResultsWithPayload re-queries points to get their payload.
func (r *ScanRepositoryQdrantImpl) fetchResultsWithPayload(ctx context.Context, collectionName string, points []*qdrant.ScoredPoint) ([]*qdrant.ScoredPoint, error) {
	if len(points) == 0 {
		return points, nil
	}

	ids := make([]*qdrant.PointId, len(points))
	for i, point := range points {
		ids[i] = point.Id
	}

	// Get points with payload
	retrievedPoints, err := r.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: collectionName,
		Ids:            ids,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch points with payload: %w", err)
	}

	// Map scores back to points
	resultMap := make(map[uint64]float32)
	for _, point := range points {
		resultMap[point.Id.GetNum()] = point.Score
	}

	// Create scored points with payload
	results := make([]*qdrant.ScoredPoint, len(retrievedPoints))
	for i, point := range retrievedPoints {
		results[i] = &qdrant.ScoredPoint{
			Id:      point.Id,
			Score:   resultMap[point.Id.GetNum()],
			Payload: point.Payload,
		}
	}

	return results, nil
}

// processSearchResults processes search results and returns component groups.
func (r *ScanRepositoryQdrantImpl) processSearchResults(searchResp []*qdrant.ScoredPoint) ([]entities.ComponentGroup, error) {
	allResults := make([]entities.SearchResult, 0, len(searchResp))

	if len(searchResp) == 0 {
		return []entities.ComponentGroup{}, nil
	}

	// Convert all points to results
	for i, point := range searchResp {
		result := r.convertPointToResult(point)
		// Avoid results with higher distances. Offset of 3 to fix "0" score comparation. Ex 0 vs 1.
		scoreThreshold := searchResp[0].Score*1.1 + 3.0
		if result.Score > scoreThreshold {
			fmt.Printf("Filtering out results starting at index %d. Best score: %f, threshold: %f, current score: %f\n", i, searchResp[0].Score, scoreThreshold, result.Score)
			break
		}

		allResults = append(allResults, result)
		if result.Rank < 2 {
			break // Stop if the component is popular enough
		}
	}

	fmt.Printf("processSearchResults: converted %d points to %d results\n", len(searchResp), len(allResults))

	// Group by component
	purlGroups := r.groupByPurl(allResults)

	fmt.Printf("processSearchResults: grouped into %d component groups\n", len(purlGroups))

	return purlGroups, nil
}

// GetCollectionStats returns statistics for a given collection.
func (r *ScanRepositoryQdrantImpl) GetCollectionStats(ctx context.Context, collectionName string) (*CollectionStats, error) {
	info, err := r.client.GetCollectionInfo(ctx, collectionName)
	if err != nil {
		return nil, errors.New("failed to get collection info: " + err.Error())
	}

	pointsCount := uint64(0)
	if info.PointsCount != nil {
		pointsCount = *info.PointsCount
	}

	return &CollectionStats{
		Name:          collectionName,
		Status:        info.Status.String(),
		PointsCount:   pointsCount,
		SegmentsCount: info.SegmentsCount,
	}, nil
}

// CollectionExists checks if a collection exists.
func (r *ScanRepositoryQdrantImpl) CollectionExists(ctx context.Context, collectionName string) (bool, error) {
	exists, err := r.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return false, errors.New("failed to check collection exists: " + err.Error())
	}
	return exists, nil
}

// GetAllSupportedCollections returns all supported collection names.
func (r *ScanRepositoryQdrantImpl) GetAllSupportedCollections() []string {
	return entities.GetAllSupportedCollections()
}

// hexSimhashToVector converts hex hash string to vector.
func (r *ScanRepositoryQdrantImpl) hexSimhashToVector(hexHashString string, bits int) ([]float32, error) {
	if hexHashString == "" {
		return nil, fmt.Errorf("input hex string cannot be empty")
	}

	uintValue, err := strconv.ParseUint(hexHashString, 16, bits)
	if err != nil {
		return nil, fmt.Errorf("could not parse hex string '%s': %w", hexHashString, err)
	}

	formatString := fmt.Sprintf("%%0%db", bits)
	binaryString := fmt.Sprintf(formatString, uintValue)

	if len(binaryString) != bits {
		return nil, fmt.Errorf("internal error: formatted binary string length (%d) does not match expected bits (%d)", len(binaryString), bits)
	}

	vector := make([]float32, bits)
	for i, bitRune := range binaryString {
		if bitRune == '1' {
			vector[i] = 1.0
		} else {
			vector[i] = 0.0
		}
	}

	return vector, nil
}

// convertPointToResult converts a Qdrant ScoredPoint to SearchResult.
func (r *ScanRepositoryQdrantImpl) convertPointToResult(point *qdrant.ScoredPoint) entities.SearchResult {
	result := entities.SearchResult{
		Score: point.Score,
		ID:    point.Id.GetNum(),
	}

	// Extract payload fields if they exist
	//nolint:nestif // Payload extraction requires nested checks
	if point.Payload != nil {
		if val, exists := point.Payload["vendor"]; exists {
			result.Vendor = val.GetStringValue()
		}
		if val, exists := point.Payload["component"]; exists {
			result.Component = val.GetStringValue()
		}
		if val, exists := point.Payload["purl"]; exists {
			result.Purl = val.GetStringValue()
		}
		if val, exists := point.Payload["version"]; exists {
			result.Version = val.GetStringValue()
		}
		if val, exists := point.Payload["rank"]; exists {
			result.Rank = int(val.GetIntegerValue())
		}
		if val, exists := point.Payload["release_date"]; exists {
			result.ReleaseDate = val.GetStringValue()
		}
		if val, exists := point.Payload["url_md5"]; exists {
			result.URLMD5 = val.GetStringValue()
		}
		if val, exists := point.Payload["license"]; exists {
			result.License = val.GetStringValue()
		}
	}

	return result
}

// groupByPurl groups search results by PURL and sorts by score and rank.
func (r *ScanRepositoryQdrantImpl) groupByPurl(results []entities.SearchResult) []entities.ComponentGroup {
	if len(results) == 0 {
		return []entities.ComponentGroup{}
	}

	// Sort all results by score (lower is better for distance) then by rank (lower is better)
	sort.Slice(results, func(i, j int) bool {
		// If scores are very similar (within 10%), prefer lower rank. Offset of 2 to fix "0" score comparation. Ex 0 vs 1.
		scoreDiff := results[j].Score - results[i].Score
		if scoreDiff <= 0.2*results[i].Score+2 {
			return results[i].Rank < results[j].Rank
		}
		// Otherwise, prefer lower score (closer distance)
		return results[i].Score < results[j].Score
	})

	// Group results by PURL
	purlGroups := make(map[string][]entities.SearchResult)
	var orderedPurls []string // Keep track of PURL order
	for i := range results {
		result := &results[i]
		if _, exists := purlGroups[result.Purl]; !exists {
			orderedPurls = append(orderedPurls, result.Purl)
		}
		purlGroups[result.Purl] = append(purlGroups[result.Purl], *result)
	}

	componentGroups := make([]entities.ComponentGroup, 0, len(orderedPurls))
	// Process groups in the order of their first appearance (which is sorted by score and rank)
	for i, purl := range orderedPurls {
		group := purlGroups[purl]

		// Sort versions within the group by score
		sort.Slice(group, func(i, j int) bool {
			return group[i].Score < group[j].Score
		})

		var versions []entities.Version
		for j := range group {
			item := &group[j]
			versions = append(versions, entities.Version{
				Version:     item.Version,
				Score:       distanceToScore(item.Score),
				ReleaseDate: item.ReleaseDate,
				URLMD5:      item.URLMD5,
				License:     item.License,
			})
		}

		// Safe int to int32 conversion
		rank := group[0].Rank
		if rank > 2147483647 {
			rank = 2147483647
		} else if rank < -2147483648 {
			rank = -2147483648
		}
		order := i + 1
		if order > 2147483647 {
			order = 2147483647
		}

		componentGroups = append(componentGroups, entities.ComponentGroup{
			PURL:     purl,
			Name:     group[0].Component,
			Vendor:   group[0].Vendor,
			Versions: versions,
			Rank:     int32(rank),  // #nosec G115 -- bounds checked above
			Order:    int32(order), // #nosec G115 -- bounds checked above
		})
	}

	return componentGroups
}

// Convert an absolute distance to matching score [0,1].
func distanceToScore(distance float32) float32 {
	const k = 0.065 // -ln(0.2) / 30
	return float32(math.Exp(-k * float64(distance)))
}

// ScoreToDistance converts a matching score [0,1] to absolute distance.
func ScoreToDistance(match float32) float32 {
	const k = 0.065 // -ln(0.2) / 30
	return float32(-math.Log(float64(match)) / k)
}
