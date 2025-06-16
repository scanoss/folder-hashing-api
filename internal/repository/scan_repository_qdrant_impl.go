package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"

	"slices"

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
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, topK uint64) ([]entities.ComponentGroup, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Info("Starting language-based search with nested sequential approach")

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
	dirVector, err := r.hexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, errors.New("failed to convert directory hash to vector: " + err.Error())
	}
	nameVector, err := r.hexSimhashToVector(nameHash, VectorDim)
	if err != nil {
		return nil, errors.New("failed to convert name hash to vector: " + err.Error())
	}
	contentVector, err := r.hexSimhashToVector(contentHash, VectorDim)
	if err != nil {
		return nil, errors.New("failed to convert content hash to vector: " + err.Error())
	}

	var filters *qdrant.Filter
	mustConditions := []*qdrant.Condition{}
	mustNotConditions := []*qdrant.Condition{}
	shouldConditions := []*qdrant.Condition{}

	// Conditions to prioritize rank < 5
	th := float64(5.0)
	rankConditions := []*qdrant.Condition{
		{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "rank",
					Range: &qdrant.Range{
						Lt: &th,
					},
				},
			},
		},
	}

	allowedCategories := []*qdrant.Condition{
		qdrant.NewMatch("category", "github_popular"),
		qdrant.NewMatch("category", "github"),
	}
	shouldConditions = append(shouldConditions, allowedCategories...)
	shouldConditions = append(shouldConditions, rankConditions...)

	mustNotConditions = append(mustNotConditions, qdrant.NewMatch("category", "forks"))
	mustNotConditions = append(mustNotConditions, qdrant.NewMatch("category", "common"))
	mustConditions = append(mustConditions, rankConditions...)

	filters = &qdrant.Filter{
		Must:    mustConditions,
		MustNot: mustNotConditions,
		Should:  shouldConditions,
	}

	filters2 := &qdrant.Filter{
		Must:    mustConditions,
		MustNot: mustNotConditions,
	}

	// Contents has minimal influence - only as a pre-filter
	s.Info("Executing nested prefetch query: names -> dirs -> contents (minimal influence)")
	nestedQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Prefetch: []*qdrant.PrefetchQuery{
			{
				// Level 3: Prefetch priority with filter2
				Prefetch: []*qdrant.PrefetchQuery{
					{
						// Level 2: Filter by dirs of the candidates of names
						Prefetch: []*qdrant.PrefetchQuery{
							{
								// Level 1: Search candidates by names with filter2 (high priority)
								Query:          qdrant.NewQuery(nameVector...),
								Using:          qdrant.PtrOf("names"),
								Filter:         filters2,                  // Filter priority
								Limit:          qdrant.PtrOf(uint64(100)), // Generous limit for filter2
								ScoreThreshold: qdrant.PtrOf(float32(30.0)),
							},
						},
						Query:          qdrant.NewQuery(dirVector...),
						Using:          qdrant.PtrOf("dirs"),
						Limit:          qdrant.PtrOf(uint64(50)),
						ScoreThreshold: qdrant.PtrOf(float32(10.0)),
					},
				},
				Query:          qdrant.NewQuery(contentVector...),
				Using:          qdrant.PtrOf("contents"),
				Limit:          qdrant.PtrOf(uint64(25)), // Less candidates than the priority group
				ScoreThreshold: qdrant.PtrOf(float32(50.0)),
			},
			{
				// Secondary prefetch with filter1 (filler)
				Prefetch: []*qdrant.PrefetchQuery{
					{
						Prefetch: []*qdrant.PrefetchQuery{
							{
								Query:          qdrant.NewQuery(nameVector...),
								Using:          qdrant.PtrOf("names"),
								Filter:         filters,                     // Secondary filter
								Limit:          qdrant.PtrOf(uint64(200)),   // Less limit
								ScoreThreshold: qdrant.PtrOf(float32(15.0)), // More strict threshold
							},
						},
						Query:          qdrant.NewQuery(dirVector...),
						Using:          qdrant.PtrOf("dirs"),
						Limit:          qdrant.PtrOf(uint64(100)),
						ScoreThreshold: qdrant.PtrOf(float32(50.0)),
					},
				},
				Query:          qdrant.NewQuery(contentVector...),
				Using:          qdrant.PtrOf("contents"),
				Limit:          qdrant.PtrOf(uint64(50)), // More candidates than the secondary group
				ScoreThreshold: qdrant.PtrOf(float32(50.0)),
			},
		},
		// Final query: rank by names
		Query:       qdrant.NewQuery(nameVector...),
		Using:       qdrant.PtrOf("names"),
		WithPayload: qdrant.NewWithPayload(true),
		WithVectors: qdrant.NewWithVectors(false),
		Limit:       qdrant.PtrOf(topK),
		// No final filter to allow both groups
	}
	searchResp, err := r.client.Query(ctx, nestedQuery)
	if err != nil {
		return nil, fmt.Errorf("error performing nested prefetch search in %s: %v", collectionName, err)
	}

	// Collect all results and their scores
	var allResults []entities.SearchResult
	var scores []float32

	for _, point := range searchResp {
		result := r.convertPointToResult(point)
		allResults = append(allResults, result)
		scores = append(scores, point.Score)
	}

	if len(scores) == 0 {
		s.Info("No final results found")
		return []entities.ComponentGroup{}, nil
	}

	// Sort scores to analyze distribution
	slices.Sort(scores)

	s.Info("Nested sequential search found %d quality results in %s", len(allResults), collectionName)

	// Group by component
	componentGroups := r.groupByComponent(allResults)

	s.Info("Nested sequential search completed: %d results grouped into %d components", len(allResults), len(componentGroups))
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
			if versionResult.Score < group.BestMatch.Score {
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
			return group.AllVersions[i].Score < group.AllVersions[j].Score
		})

		// Create other versions list (excluding best match)
		for _, version := range group.AllVersions {
			if version.Version != group.BestMatch.Version || version.Score != group.BestMatch.Score {
				group.OtherVersions = append(group.OtherVersions, version.Version)
			}
		}

		groupSlice = append(groupSlice, *group)
	}

	// Sort groups by best match score
	sort.Slice(groupSlice, func(i, j int) bool {
		if math.Abs(float64(groupSlice[i].BestMatch.Score-groupSlice[j].BestMatch.Score)) < 5 && groupSlice[i].Rank < groupSlice[j].Rank {
			return true
		} else {
			return groupSlice[i].BestMatch.Score < groupSlice[j].BestMatch.Score
		}
	})

	return groupSlice
}

func (r *ScanRepositoryQdrantImpl) getPointIds(results []*qdrant.ScoredPoint) []*qdrant.PointId {
	var ids []*qdrant.PointId
	for _, point := range results {
		if point.Id != nil {
			ids = append(ids, point.Id)
		}
	}
	return ids
}
