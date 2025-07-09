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
// Uses a two-phase approach: broad search followed by refinement
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, topK uint64, rankThreshold int) ([]entities.ComponentGroup, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Info("Starting two-phase language-based search")

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

	// Execute two-phase search
	searchResp, err := r.executeTwoPhaseSearch(ctx, collectionName, nameVector, dirsVector, contentVector, rankThreshold)
	if err != nil {
		return nil, fmt.Errorf("error performing two-phase search in %s: %v", collectionName, err)
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

// executeTwoPhaseSearch executes a two-phase search strategy
func (r *ScanRepositoryQdrantImpl) executeTwoPhaseSearch(ctx context.Context, collectionName string, nameVector, dirsVector, contentVector []float32, rankThreshold int) ([]*qdrant.ScoredPoint, error) {
	// Create rank filters
	rankThresholdConditions := r.createRangeRankCondition(rankThreshold)
	rankThresholdFilter := &qdrant.Filter{Must: rankThresholdConditions}

	exactRank1Conditions := r.createExactRankCondition(1)
	exactRank1Filter := &qdrant.Filter{Must: exactRank1Conditions}

	// Phase 1: Broad search with high limits
	// CORRECT STRUCTURE: Start with names, then filter by contents, then by dirs

	// First, search by names to get all candidates
	nameQuery1 := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(nameVector...),
		Using:          qdrant.PtrOf("names"),
		Filter:         rankThresholdFilter,
		WithPayload:    qdrant.NewWithPayload(false),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(40000)), // Get all name candidates
	}

	// Also search for rank 1 components specifically
	nameQueryRank1 := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(nameVector...),
		Using:          qdrant.PtrOf("names"),
		Filter:         exactRank1Filter,
		WithPayload:    qdrant.NewWithPayload(false),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(5000)), // Get rank 1 candidates
	}

	// Execute both name searches
	nameResults1, err := r.client.Query(ctx, nameQuery1)
	if err != nil {
		return nil, fmt.Errorf("phase 1 name search failed: %w", err)
	}

	nameResultsRank1, err := r.client.Query(ctx, nameQueryRank1)
	if err != nil {
		return nil, fmt.Errorf("phase 1 rank 1 name search failed: %w", err)
	}

	// Combine and deduplicate results
	combinedIDs := make(map[uint64]float32)
	for _, point := range nameResults1 {
		combinedIDs[point.Id.GetNum()] = point.Score
	}
	for _, point := range nameResultsRank1 {
		if existingScore, exists := combinedIDs[point.Id.GetNum()]; !exists || point.Score < existingScore {
			combinedIDs[point.Id.GetNum()] = point.Score
		}
	}

	// Convert to IDs for next phase
	nameIDs := make([]uint64, 0, len(combinedIDs))
	for id := range combinedIDs {
		nameIDs = append(nameIDs, id)
	}

	// If we have no results, return empty
	if len(nameIDs) == 0 {
		return []*qdrant.ScoredPoint{}, nil
	}

	// Phase 1.2: Filter by contents
	contentFilter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(nameIDs),
					},
				},
			},
		},
	}

	contentQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(contentVector...),
		Using:          qdrant.PtrOf("contents"),
		Filter:         contentFilter,
		WithPayload:    qdrant.NewWithPayload(false),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(25000)), // Filter to 15000 by content
	}

	contentResults, err := r.client.Query(ctx, contentQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 1 content search failed: %w", err)
	}

	// Extract IDs from content results
	contentIDs := make([]uint64, len(contentResults))
	for i, point := range contentResults {
		contentIDs[i] = point.Id.GetNum()
	}

	// Phase 1.3: Final filter by dirs
	dirsFilter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(contentIDs),
					},
				},
			},
		},
	}

	dirsQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(dirsVector...),
		Using:          qdrant.PtrOf("dirs"),
		Filter:         dirsFilter,
		WithPayload:    qdrant.NewWithPayload(false),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(10000)), // Reduce to 10000 candidates
	}

	phase1Results, err := r.client.Query(ctx, dirsQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 1 dirs search failed: %w", err)
	}

	// If we have 10 or fewer results, return them directly
	if len(phase1Results) <= 10 {
		return r.fetchResultsWithPayload(ctx, collectionName, phase1Results)
	}

	// Phase 2: Refined search on the 10000 candidates
	// Extract IDs from phase 1 results
	phase1IDs := make([]uint64, len(phase1Results))
	for i, point := range phase1Results {
		phase1IDs[i] = point.Id.GetNum()
	}

	// Phase 2.1: Re-filter by names with lower limit
	phase2NameFilter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(phase1IDs),
					},
				},
			},
		},
	}

	phase2NameQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(nameVector...),
		Using:          qdrant.PtrOf("names"),
		Filter:         phase2NameFilter,
		WithPayload:    qdrant.NewWithPayload(false),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(5000)), // More selective
	}

	phase2NameResults, err := r.client.Query(ctx, phase2NameQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 2 name search failed: %w", err)
	}

	// Extract IDs for content filtering
	phase2NameIDs := make([]uint64, len(phase2NameResults))
	for i, point := range phase2NameResults {
		phase2NameIDs[i] = point.Id.GetNum()
	}

	// Phase 2.2: Filter by contents
	phase2ContentFilter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(phase2NameIDs),
					},
				},
			},
		},
	}

	phase2ContentQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(dirsVector...),
		Using:          qdrant.PtrOf("dirs"),
		Filter:         phase2ContentFilter,
		WithPayload:    qdrant.NewWithPayload(false),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(100)), // Aggressive filtering
	}

	phase2ContentResults, err := r.client.Query(ctx, phase2ContentQuery)
	if err != nil {
		return nil, fmt.Errorf("phase 2 content search failed: %w", err)
	}

	// Extract IDs for final dirs filtering
	phase2ContentIDs := make([]uint64, len(phase2ContentResults))
	for i, point := range phase2ContentResults {
		phase2ContentIDs[i] = point.Id.GetNum()
	}

	// Phase 2.3: Final filter by dirs with payload
	phase2DirsFilter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: r.convertIDsToPointIDs(phase2ContentIDs),
					},
				},
			},
		},
	}

	finalQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(contentVector...),
		Using:          qdrant.PtrOf("contents"),
		Filter:         phase2DirsFilter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
		Limit: qdrant.PtrOf(uint64(20)), // Final 10 results
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
