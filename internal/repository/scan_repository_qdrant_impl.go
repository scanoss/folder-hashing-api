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
// Uses single query with 4-branch rank-aware prefetch structure to prioritize rank 1 entries
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, topK uint64, rankThreshold int) ([]entities.ComponentGroup, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Info("Starting language-based search with content-enhanced prefetching")

	// Determine target collection based on primary language
	collectionName := entities.GetCollectionNameFromLanguageExtensions(langExt)
	s.Info("Using collection: %s for language extensions: %v", collectionName, langExt)

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

	searchResp, err := r.executeRankAwareHybridQuery(ctx, collectionName, nameVector, dirsVector, contentVector, rankThreshold)
	if err != nil {
		return nil, fmt.Errorf("error performing rank-aware search in %s: %v", collectionName, err)
	}

	return r.processSearchResults(searchResp)
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

// executeRankAwareHybridQuery executes a hybrid query with rank-aware prefetch structure
// Uses 4 branches to give content hash more weight for tie-breaking and exact match detection
func (r *ScanRepositoryQdrantImpl) executeRankAwareHybridQuery(ctx context.Context, collectionName string, nameVector, dirsVector, contentVector []float32, rankThreshold int) ([]*qdrant.ScoredPoint, error) {
	// Create rank conditions
	rank1Conditions := r.createExactRankCondition(1)
	rankThresholdConditions := r.createRangeRankCondition(rankThreshold)

	// Create filters
	rank1Filter := &qdrant.Filter{Must: rank1Conditions}
	rankThresholdFilter := &qdrant.Filter{Must: rankThresholdConditions}

	// Build 4-branch prefetch structure for enhanced content weighting
	// This gives content hash multiple pathways to influence scoring
	prefetchQueries := []*qdrant.PrefetchQuery{
		// Branch A: Names-first with rank 1 priority (discovery)
		{
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query:  qdrant.NewQuery(dirsVector...),
					Using:  qdrant.PtrOf("dirs"),
					Filter: rank1Filter,
				},
				{
					Query:  qdrant.NewQuery(contentVector...),
					Using:  qdrant.PtrOf("contents"),
					Filter: rank1Filter,
				},
			},
			Query:  qdrant.NewQuery(nameVector...),
			Using:  qdrant.PtrOf("names"),
			Filter: rank1Filter,
		},
		// Branch B: Names-first with rank <= rankThreshold (broader discovery)
		{
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query:  qdrant.NewQuery(dirsVector...),
					Using:  qdrant.PtrOf("dirs"),
					Filter: rankThresholdFilter,
				},
				{
					Query:  qdrant.NewQuery(contentVector...),
					Using:  qdrant.PtrOf("contents"),
					Filter: rankThresholdFilter,
				},
			},
			Query:  qdrant.NewQuery(nameVector...),
			Using:  qdrant.PtrOf("names"),
			Filter: rankThresholdFilter,
		},
		// Branch C: Content-first with rank 1 (exact match emphasis)
		{
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query:  qdrant.NewQuery(nameVector...),
					Using:  qdrant.PtrOf("names"),
					Filter: rank1Filter,
				},
			},
			Query:  qdrant.NewQuery(contentVector...),
			Using:  qdrant.PtrOf("contents"),
			Filter: rank1Filter,
		},
		// Branch D: Content-first with rank <= rankThreshold (content-driven tie-breaking)
		{
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query:  qdrant.NewQuery(nameVector...),
					Using:  qdrant.PtrOf("names"),
					Filter: rankThresholdFilter,
				},
			},
			Query:  qdrant.NewQuery(contentVector...),
			Using:  qdrant.PtrOf("contents"),
			Filter: rankThresholdFilter,
		},
	}

	// Execute hybrid query with RRF fusion
	hybridQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQueryFusion(qdrant.Fusion_RRF),
		Prefetch:       prefetchQueries,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Filter:         rankThresholdFilter, // Overall filter for rank <= rankThreshold
	}

	return r.client.Query(ctx, hybridQuery)
}

// processSearchResults processes search results and returns component groups
func (r *ScanRepositoryQdrantImpl) processSearchResults(searchResp []*qdrant.ScoredPoint) ([]entities.ComponentGroup, error) {
	var allResults []entities.SearchResult

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

// groupByPurl groups search results by PURL
func (r *ScanRepositoryQdrantImpl) groupByPurl(results []entities.SearchResult) []entities.ComponentGroup {
	if len(results) == 0 {
		return []entities.ComponentGroup{}
	}

	// Sort all results by score (higher is better) then by rank (lower is better)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Rank < results[j].Rank
		}
		return results[i].Score > results[j].Score
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
	// Process groups in the order of their first appearance (which is sorted by score)
	for i, purl := range orderedPurls {
		group := purlGroups[purl]

		// Sort versions within the group by score (higher is better)
		sort.Slice(group, func(i, j int) bool {
			return group[i].Score > group[j].Score
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
