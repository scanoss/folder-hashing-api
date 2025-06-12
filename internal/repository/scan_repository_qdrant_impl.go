package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	domainErrors "github.com/scanoss/folder-hashing-api/internal/domain/errors"
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
		return nil, domainErrors.NewRepositoryFailureError("connect", err)
	}

	return &ScanRepositoryQdrantImpl{
		client: client,
		config: config,
	}, nil
}

// SearchByHashes performs a search using directory, name, and content hashes
func (r *ScanRepositoryQdrantImpl) SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string,
	langExt entities.LanguageExtensions, topK uint64) ([]entities.ComponentGroup, error) {

	log.Printf("Starting language-based search with RRF fusion")

	// Determine target collection based on primary language
	collectionName := entities.GetCollectionNameFromLanguageExtensions(langExt)
	log.Printf("Using collection: %s for language extensions: %v", collectionName, langExt)

	// Set context with timeout
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Check if collection exists
	exists, err := r.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, domainErrors.NewRepositoryFailureError("check_collection", err)
	}
	if !exists {
		log.Printf("Collection %s does not exist, falling back to misc_collection", collectionName)
		collectionName = "misc_collection"

		// Check if misc collection exists
		exists, err = r.client.CollectionExists(ctx, collectionName)
		if err != nil || !exists {
			return nil, domainErrors.NewCollectionNotFoundError(collectionName)
		}
	}

	// Convert hashes to vectors
	dirVector, err := hexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, domainErrors.NewInvalidHashError("dir", dirHash)
	}
	nameVector, err := hexSimhashToVector(nameHash, VectorDim)
	if err != nil {
		return nil, domainErrors.NewInvalidHashError("name", nameHash)
	}
	contentVector, err := hexSimhashToVector(contentHash, VectorDim)
	if err != nil {
		return nil, domainErrors.NewInvalidHashError("content", contentHash)
	}

	var filters *qdrant.Filter
	mustConditions := []*qdrant.Condition{}
	mustNotConditions := []*qdrant.Condition{}
	shouldConditions := []*qdrant.Condition{}

	if len(langExt) > 0 {
		langExtConditions := r.buildLanguageExtensionFilter(langExt, 30.0)
		if len(langExtConditions) > 0 {
			mustConditions = append(mustConditions, langExtConditions...)
		}
	}

	// We want to get only 'github_popular' or 'github' category results
	allowedCategories := []*qdrant.Condition{
		qdrant.NewMatch("category", "github_popular"),
		qdrant.NewMatch("category", "github"),
		qdrant.NewMatch("category", "common"),
	}
	shouldConditions = append(shouldConditions, allowedCategories...)

	mustNotConditions = append(mustNotConditions, qdrant.NewMatch("category", "forks"))

	filters = &qdrant.Filter{
		Must:    mustConditions,
		MustNot: mustNotConditions,
		Should:  shouldConditions,
	}

	prefetchQueries := []*qdrant.PrefetchQuery{
		{
			// Names vector query
			Query: qdrant.NewQuery(nameVector...),
			Using: qdrant.PtrOf("names"),
		},
		{
			// Dirs vector query
			Query: qdrant.NewQuery(dirVector...),
			Using: qdrant.PtrOf("dirs"),
		},
		{
			// Contents vector query
			Query: qdrant.NewQuery(contentVector...),
			Using: qdrant.PtrOf("contents"),
		},
	}

	// Create hybrid query with weighted fusion
	hybridQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQueryFusion(qdrant.Fusion_RRF),
		Prefetch:       prefetchQueries,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Filter:         filters,
	}

	searchResp, err := r.client.Query(ctx, hybridQuery)
	if err != nil {
		return nil, domainErrors.NewRepositoryFailureError("search", err)
	}

	// First, collect all results and their scores
	var allResults []entities.SearchResult
	var scores []float32

	for _, point := range searchResp {
		result := r.convertPointToResult(point)
		allResults = append(allResults, result)
		scores = append(scores, point.Score)
	}

	if len(scores) == 0 {
		log.Printf("No search results found")
		return []entities.ComponentGroup{}, nil
	}

	// Sort scores to analyze distribution
	sort.Slice(scores, func(i, j int) bool {
		return scores[i] > scores[j] // Sort descending
	})

	log.Printf("RRF hybrid search found %d quality results in %s after filtering", len(allResults), collectionName)

	// Group by component
	componentGroups := r.groupByComponent(allResults)

	log.Printf("RRF hybrid search completed: %d results grouped into %d components", len(allResults), len(componentGroups))
	return componentGroups, nil
}

// GetCollectionStats returns statistics for a given collection
func (r *ScanRepositoryQdrantImpl) GetCollectionStats(ctx context.Context, collectionName string) (*CollectionStats, error) {
	info, err := r.client.GetCollectionInfo(ctx, collectionName)
	if err != nil {
		return nil, domainErrors.NewRepositoryFailureError("get_collection_info", err)
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
		return false, domainErrors.NewRepositoryFailureError("check_collection_exists", err)
	}
	return exists, nil
}

// GetAllSupportedCollections returns all supported collection names
func (r *ScanRepositoryQdrantImpl) GetAllSupportedCollections() []string {
	return entities.GetAllSupportedCollections()
}

// hexSimhashToVector converts hex hash string to vector
func hexSimhashToVector(hexHashString string, bits int) ([]float32, error) {
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

// buildLanguageExtensionFilter creates Qdrant filters for language extension similarity
func (r *ScanRepositoryQdrantImpl) buildLanguageExtensionFilter(queryLangExt entities.LanguageExtensions, tolerancePercent float32) []*qdrant.Condition {
	if len(queryLangExt) == 0 {
		return nil
	}

	var conditions []*qdrant.Condition

	// For each language in query, create range filters
	for extension, count := range queryLangExt {
		if extension == "" {
			continue
		}
		// If extension is not in IndexedLangExtensions, skip it
		if !slices.Contains(entities.IndexedLangExtensions, extension) {
			continue
		}
		if count <= 0 {
			continue
		}

		// Calculate tolerance range (e.g., ±30% of the count)
		tolerance := float32(count) * tolerancePercent / 100.0
		minCount := int64(math.Max(0, float64(count)-float64(tolerance)))
		maxCount := int64(float64(count) + float64(tolerance))

		// Create range condition for this language
		condition := &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "language_extensions." + extension,
					Range: &qdrant.Range{
						Gte: qdrant.PtrOf(float64(minCount)),
						Lte: qdrant.PtrOf(float64(maxCount)),
					},
				},
			},
		}
		conditions = append(conditions, condition)
	}

	return conditions
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
		key := result.Component
		if key == "" {
			key = "unknown"
		}

		versionResult := entities.VersionResult{
			Version:            result.Version,
			Score:              result.Score,
			URL:                result.URL,
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

	// Sort groups by best match score (higher is better because of RRF)
	sort.Slice(groupSlice, func(i, j int) bool {
		return groupSlice[i].BestMatch.Score > groupSlice[j].BestMatch.Score
	})

	return groupSlice
}
