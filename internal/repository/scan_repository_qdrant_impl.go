package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/qdrant/go-client/qdrant"
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection
)

type QdrantConfig struct {
	Host string
	Port int
}

// scanRepository implements the ScanRepository interface using Qdrant
type ScanRepositoryQdrantImpl struct {
	client *qdrant.Client
	config QdrantConfig
}

// NewScanRepository creates a new Qdrant-based scan repository
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

// SearchByHashes performs a search using directory, name, and content hashes
// Uses single query with multi-stage prefetch structure
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, topK uint64, rankThreshold int) ([]entities.ComponentGroup, error) {
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
		return nil, fmt.Errorf("error performing optimized search in %s: %v", collectionName, err)
	}

	return r.processSearchResults(searchResp)
}

// createRangeRankCondition creates a condition for rank range matching (rank <= maxRank)
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

// createExactRankCondition creates a condition for exact rank matching
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

// executeOptimizedQuery executes a single query with multi-stage prefetch
// The key insight: we use multiple prefetch branches at the SAME level to get different candidate sets
// Then use nested prefetches to apply progressive filtering
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
				Limit: qdrant.PtrOf(uint64(40000)), // Very broad initial search
			},
			// Branch 2: Ensure rank 1 components are included
			{
				Query:  qdrant.NewQuery(nameVector...),
				Using:  qdrant.PtrOf("names"),
				Filter: exactRank1Filter,
				Params: &qdrant.SearchParams{
					Exact: qdrant.PtrOf(true),
				},
				Limit: qdrant.PtrOf(uint64(5000)), // Dedicated search for popular components
			},
		},
		// After prefetch collects candidates by name, apply content filtering
		Query:  qdrant.NewQuery(contentVector...),
		Using:  qdrant.PtrOf("contents"),
		Filter: rankThresholdFilter,
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit:       qdrant.PtrOf(uint64(25000)), // Filter down by content similarity
		WithPayload: qdrant.NewWithPayload(false),
		WithVectors: qdrant.NewWithVectors(false),
	}

	// Execute the first stage
	stage1Results, err := r.client.Query(ctx, hybridQuery)
	if err != nil {
		return nil, fmt.Errorf("stage 1 search failed: %w", err)
	}

	// If we have few results, fetch payload and return
	if len(stage1Results) <= 20 {
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
		Limit:       qdrant.PtrOf(uint64(10000)), // Reduce to 10000
		WithPayload: qdrant.NewWithPayload(false),
		WithVectors: qdrant.NewWithVectors(false),
	}

	stage2Results, err := r.client.Query(ctx, stage2Query)
	if err != nil {
		return nil, fmt.Errorf("stage 2 search failed: %w", err)
	}

	// If we have 20 or fewer results, return them
	if len(stage2Results) <= 20 {
		return r.fetchResultsWithPayload(ctx, collectionName, stage2Results)
	}

	// Stage 3: Final refinement using nested prefetch for the best results
	stage2IDs := make([]uint64, len(stage2Results))
	for i, point := range stage2Results {
		stage2IDs[i] = point.Id.GetNum()
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

	// Final query with nested prefetch for optimal ranking
	finalQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Prefetch: []*qdrant.PrefetchQuery{
			// Re-rank by names within our candidate set
			{
				Query:  qdrant.NewQuery(nameVector...),
				Using:  qdrant.PtrOf("names"),
				Filter: finalFilter,
				Params: &qdrant.SearchParams{
					Exact: qdrant.PtrOf(true),
				},
				Limit: qdrant.PtrOf(uint64(5000)),
				Prefetch: []*qdrant.PrefetchQuery{
					// Then by dirs
					{
						Query:  qdrant.NewQuery(dirsVector...),
						Using:  qdrant.PtrOf("dirs"),
						Filter: finalFilter,
						Params: &qdrant.SearchParams{
							Exact: qdrant.PtrOf(true),
						},
						Limit: qdrant.PtrOf(uint64(100)),
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
		Limit: qdrant.PtrOf(uint64(20)), // Final results
	}

	return r.client.Query(ctx, finalQuery)
}

// convertIDsToPointIDs converts uint64 IDs to PointId format
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

// fetchResultsWithPayload re-queries points to get their payload
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

// processSearchResults processes search results and returns component groups
func (r *ScanRepositoryQdrantImpl) processSearchResults(searchResp []*qdrant.ScoredPoint) ([]entities.ComponentGroup, error) {
	var allResults []entities.SearchResult

	// Convert all points to results
	for _, point := range searchResp {
		result := r.convertPointToResult(point)
		allResults = append(allResults, result)
	}

	// Group by component
	purlGroups := r.groupByPurl(allResults)

	return purlGroups, nil
}

// GetCollectionStats returns statistics for a given collection
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

// CollectionExists checks if a collection exists
func (r *ScanRepositoryQdrantImpl) CollectionExists(ctx context.Context, collectionName string) (bool, error) {
	exists, err := r.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return false, errors.New("failed to check collection exists: " + err.Error())
	}
	return exists, nil
}

// GetAllSupportedCollections returns all supported collection names
func (r *ScanRepositoryQdrantImpl) GetAllSupportedCollections() []string {
	return entities.GetAllSupportedCollections()
}

// hexSimhashToVector converts hex hash string to vector
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

// convertPointToResult converts a Qdrant ScoredPoint to SearchResult
func (r *ScanRepositoryQdrantImpl) convertPointToResult(point *qdrant.ScoredPoint) entities.SearchResult {
	result := entities.SearchResult{
		Score: point.Score,
		ID:    point.Id.GetNum(),
	}

	// Extract payload fields if they exist
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
	}

	return result
}

// groupByPurl groups search results by PURL and sorts by score and rank
func (r *ScanRepositoryQdrantImpl) groupByPurl(results []entities.SearchResult) []entities.ComponentGroup {
	if len(results) == 0 {
		return []entities.ComponentGroup{}
	}

	// Sort all results by score (lower is better for distance) then by rank (lower is better)
	sort.Slice(results, func(i, j int) bool {
		// If scores are very similar (within 10%), prefer lower rank
		scoreDiff := results[j].Score - results[i].Score
		if scoreDiff < 0.1*results[i].Score {
			return results[i].Rank < results[j].Rank
		}
		// Otherwise, prefer lower score (closer distance)
		return results[i].Score < results[j].Score
	})

	// Group results by PURL
	purlGroups := make(map[string][]entities.SearchResult)
	var orderedPurls []string // Keep track of PURL order
	for _, result := range results {
		if _, exists := purlGroups[result.Purl]; !exists {
			orderedPurls = append(orderedPurls, result.Purl)
		}
		purlGroups[result.Purl] = append(purlGroups[result.Purl], result)
	}

	var componentGroups []entities.ComponentGroup
	// Process groups in the order of their first appearance (which is sorted by score and rank)
	for i, purl := range orderedPurls {
		group := purlGroups[purl]

		// Sort versions within the group by score
		sort.Slice(group, func(i, j int) bool {
			return group[i].Score < group[j].Score
		})

		var versions []entities.Version
		for _, item := range group {
			versions = append(versions, entities.Version{
				Version: item.Version,
				Score:   item.Score,
			})
		}

		componentGroups = append(componentGroups, entities.ComponentGroup{
			PURL:     purl,
			Name:     group[0].Component,
			Vendor:   group[0].Vendor,
			Versions: versions,
			Rank:     int32(group[0].Rank), // Best rank of the group
			Order:    int32(i + 1),         // Sequential order
		})
	}

	return componentGroups
}
