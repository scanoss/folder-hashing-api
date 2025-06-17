package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, topK uint64) ([]entities.ComponentGroup, error) {
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

	searchResp, err := r.executeRankAwareHybridQuery(ctx, collectionName, nameVector, dirsVector, contentVector)
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
func (r *ScanRepositoryQdrantImpl) executeRankAwareHybridQuery(ctx context.Context, collectionName string, nameVector, dirsVector, contentVector []float32) ([]*qdrant.ScoredPoint, error) {
	// Create rank conditions
	rank1Conditions := r.createExactRankCondition(1)
	rank5Conditions := r.createRangeRankCondition(5)

	// Create filters
	rank1Filter := &qdrant.Filter{Must: rank1Conditions}
	rank5Filter := &qdrant.Filter{Must: rank5Conditions}

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
		// Branch B: Names-first with rank <= 5 (broader discovery)
		{
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query:  qdrant.NewQuery(dirsVector...),
					Using:  qdrant.PtrOf("dirs"),
					Filter: rank5Filter,
				},
				{
					Query:  qdrant.NewQuery(contentVector...),
					Using:  qdrant.PtrOf("contents"),
					Filter: rank5Filter,
				},
			},
			Query:  qdrant.NewQuery(nameVector...),
			Using:  qdrant.PtrOf("names"),
			Filter: rank5Filter,
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
		// Branch D: Content-first with rank <= 5 (content-driven tie-breaking)
		{
			Prefetch: []*qdrant.PrefetchQuery{
				{
					Query:  qdrant.NewQuery(nameVector...),
					Using:  qdrant.PtrOf("names"),
					Filter: rank5Filter,
				},
			},
			Query:  qdrant.NewQuery(contentVector...),
			Using:  qdrant.PtrOf("contents"),
			Filter: rank5Filter,
		},
	}

	// Execute hybrid query with RRF fusion
	hybridQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQueryFusion(qdrant.Fusion_RRF),
		Prefetch:       prefetchQueries,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Filter:         rank5Filter, // Overall filter for rank <= 5
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
	componentGroups := r.groupByComponent(allResults)

	return componentGroups, nil
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
		Score:    point.Score,
		ID:       point.Id.GetNum(),
		Metadata: make(map[string]any),
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
		if val, exists := point.Payload["url"]; exists {
			result.URL = val.GetStringValue()
		}
		if val, exists := point.Payload["rank"]; exists {
			result.Rank = int(val.GetIntegerValue())
		}

		// Parse language_extensions if present
		if val, exists := point.Payload["language_extensions"]; exists {
			// Try structured format first
			if structVal := val.GetStructValue(); structVal != nil {
				langExt := make(entities.LanguageExtensions)
				for lang, countVal := range structVal.Fields {
					if intVal := countVal.GetIntegerValue(); intVal != 0 {
						langExt[lang] = int32(intVal)
					}
				}
				if len(langExt) > 0 {
					result.LanguageExtensions = langExt
				}
			} else if langExtStr := val.GetStringValue(); langExtStr != "" {
				// Fallback to string format for backward compatibility
				if langExt, err := r.parseLanguageExtensions(langExtStr); err == nil {
					result.LanguageExtensions = langExt
				}
			}
		}

		// Store all payload for access to other fields
		for key, value := range point.Payload {
			switch {
			case value.GetStringValue() != "":
				result.Metadata[key] = value.GetStringValue()
			case value.GetIntegerValue() != 0:
				result.Metadata[key] = value.GetIntegerValue()
			case value.GetDoubleValue() != 0:
				result.Metadata[key] = value.GetDoubleValue()
			case value.GetBoolValue():
				result.Metadata[key] = value.GetBoolValue()
			}
		}
	}

	return result
}

// parseLanguageExtensions parses the stringified JSON language extensions
func (r *ScanRepositoryQdrantImpl) parseLanguageExtensions(langExtStr string) (entities.LanguageExtensions, error) {
	var langExt entities.LanguageExtensions
	if err := json.Unmarshal([]byte(langExtStr), &langExt); err != nil {
		log.Printf("Warning: failed to parse language_extensions '%s': %v", langExtStr, err)
		return nil, err
	}
	return langExt, nil
}

// groupByComponent groups results by component name
func (r *ScanRepositoryQdrantImpl) groupByComponent(results []entities.SearchResult) []entities.ComponentGroup {
	if len(results) == 0 {
		return []entities.ComponentGroup{}
	}

	groups := make(map[string]*entities.ComponentGroup)

	for _, result := range results {
		key := result.Purl
		if key == "" {
			key = "unknown"
		}

		versionResult := entities.VersionResult{
			Version:            result.Version,
			Score:              result.Score,
			URL:                result.URL,
			Purl:               result.Purl,
			LanguageExtensions: result.LanguageExtensions,
			Metadata:           result.Metadata,
		}

		if group, exists := groups[key]; exists {
			// Add version to existing component group
			group.AllVersions = append(group.AllVersions, versionResult)
			group.ResultCount++

			// Update best match if this version has a better score
			if versionResult.Score > group.BestMatch.Score {
				group.BestMatch = versionResult
			}
		} else {
			// Create new component group
			groups[key] = &entities.ComponentGroup{
				Component:   result.Component,
				Vendor:      result.Vendor,
				BestMatch:   versionResult,
				AllVersions: []entities.VersionResult{versionResult},
				ResultCount: 1,
				Rank:        result.Rank,
			}
		}
	}

	// Convert to slice and finalize
	var groupSlice []entities.ComponentGroup
	for _, group := range groups {
		// Sort versions within group by score (higher score is better)
		sort.Slice(group.AllVersions, func(i, j int) bool {
			return group.AllVersions[i].Score > group.AllVersions[j].Score
		})

		// Create other versions list (excluding best match)
		for _, version := range group.AllVersions {
			if version.Version != group.BestMatch.Version || version.Score != group.BestMatch.Score {
				group.OtherVersions = append(group.OtherVersions, version.Version)
			}
		}

		groupSlice = append(groupSlice, *group)
	}

	sort.Slice(groupSlice, func(i, j int) bool {
		return groupSlice[i].BestMatch.Score > groupSlice[j].BestMatch.Score
	})

	return groupSlice
}
