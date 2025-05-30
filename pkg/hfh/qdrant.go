package hfh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection

	// Manhattan distance thresholds for binary vectors (lower scores = better matches)
	EXACT_MATCH_THRESHOLD       = 0  // Perfect match (identical vectors)
	HIGH_SIMILARITY_THRESHOLD   = 3  // 1-3 bit differences (stricter)
	MEDIUM_SIMILARITY_THRESHOLD = 8  // 4-8 bit differences (stricter)
	LOW_SIMILARITY_THRESHOLD    = 15 // 9-15 bit differences (stricter)
)

// QdrantConfig holds Qdrant connection configuration for single collection approach
type QdrantConfig struct {
	Host           string
	Port           int
	CollectionName string
}

// NewQdrantConfig creates a new QdrantConfig
func NewQdrantConfig(host string, port int, collectionName string) QdrantConfig {
	return QdrantConfig{
		Host:           host,
		Port:           port,
		CollectionName: collectionName,
	}
}

// LanguageExtensions represents parsed language extension counts
type LanguageExtensions map[string]int32

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Distance           float32                `json:"distance"`
	ID                 uint64                 `json:"id"`
	Vendor             string                 `json:"vendor"`
	Component          string                 `json:"component"`
	Version            string                 `json:"version"`
	URL                string                 `json:"url"`
	LanguageExtensions LanguageExtensions     `json:"language_extensions,omitempty"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// ComponentGroup represents grouped results by component name
type ComponentGroup struct {
	Component     string          `json:"component"`
	Vendor        string          `json:"vendor"`
	BestMatch     VersionResult   `json:"best_match"`
	OtherVersions []string        `json:"other_versions,omitempty"`
	AllVersions   []VersionResult `json:"all_versions,omitempty"`
	ResultCount   int             `json:"result_count"`
}

// VersionResult represents a version-specific result within a component group
type VersionResult struct {
	Version            string                 `json:"version"`
	Distance           float32                `json:"distance"`
	URL                string                 `json:"url,omitempty"`
	LanguageExtensions LanguageExtensions     `json:"language_extensions,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

func HexSimhashToVector(hexHashString string, bits int) ([]float32, error) {
	if hexHashString == "" {
		return nil, fmt.Errorf("input hex string cannot be empty")
	}

	// 1. Parse the hexadecimal string into an unsigned integer (uint64).
	//    We specify base 16 and the bit size.
	uintValue, err := strconv.ParseUint(hexHashString, 16, bits)
	if err != nil {
		return nil, fmt.Errorf("could not parse hex string '%s': %w", hexHashString, err)
	}

	// 2. Create the format string for Sprintf.
	//    This will look like "%064b" for 64 bits, ensuring padding with leading zeros.
	formatString := fmt.Sprintf("%%0%db", bits)

	// 3. Format the integer value as a binary string, padded to the correct length.
	binaryString := fmt.Sprintf(formatString, uintValue)

	// 4. Ensure the resulting binary string has the exact length.
	//    (This is a safeguard, Sprintf should handle it but doesn't hurt to check).
	if len(binaryString) != bits {
		return nil, fmt.Errorf("internal error: formatted binary string length (%d) does not match expected bits (%d)", len(binaryString), bits)
	}

	// 5. Create the float vector and populate it.
	//    We pre-allocate the slice for efficiency.
	vector := make([]float32, bits)
	for i, bitRune := range binaryString {
		if bitRune == '1' {
			vector[i] = 1.0
		} else {
			vector[i] = 0.0 // Explicitly set to 0.0, though it's the default
		}
	}

	return vector, nil
}

func searchExactByPayload(client *qdrant.Client, ctx context.Context, collectionName, vectorName, hash string) ([]SearchResult, error) {
	var payloadField string

	switch vectorName {
	case "dirs":
		payloadField = "hfh_dirs_hash"
	case "names":
		payloadField = "hfh_names_hash"
	case "contents":
		payloadField = "hfh_contents_hash"
	default:
		return nil, fmt.Errorf("unknown vector name: %s", vectorName)
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: payloadField,
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Text{
								Text: hash,
							},
						},
					},
				},
			},
		},
	}

	queryResp, err := client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collectionName,
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			Exact: qdrant.PtrOf(true),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("error in exact payload search: %v", err)
	}

	var results []SearchResult
	for _, point := range queryResp {
		result := SearchResult{
			Distance: 0.0, // Exact match
			ID:       point.Id.GetNum(),
			Metadata: make(map[string]any),
		}

		// Extract payload fields
		if point.Payload != nil {
			if val, exists := point.Payload["vendor"]; exists {
				result.Vendor = val.GetStringValue()
			}
			if val, exists := point.Payload["component"]; exists {
				result.Component = val.GetStringValue()
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
					langExt := make(LanguageExtensions)
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
					if langExt, err := parseLanguageExtensions(langExtStr); err == nil {
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

		results = append(results, result)
	}

	return results, nil
}

// performSimilaritySearch performs vector similarity search, excluding already found results
func performSimilaritySearch(client *qdrant.Client, ctx context.Context, collectionName, vectorName, hash string, topK uint64, excludeResults []SearchResult) ([]SearchResult, error) {
	// Convert hash to dense vector
	queryVector, err := HexSimhashToVector(hash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	log.Printf("Performing similarity search for %s vector with hash %s", vectorName, hash)

	// Build filter to exclude ffffffffffffffff and already found results
	var filter *qdrant.Filter
	var mustNotConditions []*qdrant.Condition

	// Always exclude corrupted ffffffffffffffff hashes
	var payloadField string
	switch vectorName {
	case "dirs":
		payloadField = "hfh_dirs_hash"
	case "names":
		payloadField = "hfh_names_hash"
	case "contents":
		payloadField = "hfh_contents_hash"
	default:
		return nil, fmt.Errorf("unknown vector name: %s", vectorName)
	}

	mustNotConditions = append(mustNotConditions, &qdrant.Condition{
		ConditionOneOf: &qdrant.Condition_Field{
			Field: &qdrant.FieldCondition{
				Key: payloadField,
				Match: &qdrant.Match{
					MatchValue: &qdrant.Match_Text{
						Text: "ffffffffffffffff",
					},
				},
			},
		},
	})

	// Exclude already found results if any
	if len(excludeResults) > 0 {
		for _, excludeResult := range excludeResults {
			mustNotConditions = append(mustNotConditions, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: []*qdrant.PointId{qdrant.NewIDNum(excludeResult.ID)},
					},
				},
			})
		}
	}

	if len(mustNotConditions) > 0 {
		filter = &qdrant.Filter{
			MustNot: mustNotConditions,
		}
	}

	// Perform the search with score threshold to filter low similarity results
	queryReq := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          &vectorName,
		Limit:          qdrant.PtrOf(topK),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error performing search: %v", err)
	}

	// Convert results and filter by quality
	var results []SearchResult
	for _, point := range searchResp {
		result := convertPointToResult(point)

		// Additional quality filtering based on score
		// For manhattan distance, lower scores are better
		if result.Distance <= HIGH_SIMILARITY_THRESHOLD {
			log.Printf("High similarity match using %s: %s %s (distance: %.3f)", vectorName, result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else if result.Distance <= MEDIUM_SIMILARITY_THRESHOLD {
			log.Printf("Medium similarity match using %s: %s %s (distance: %.3f)", vectorName, result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else {
			log.Printf("Low similarity match using %s: %s %s (distance: %.3f)", vectorName, result.Component, result.Version, result.Distance)
		}
	}

	log.Printf("Found %d similarity results with distance <= %d", len(results), LOW_SIMILARITY_THRESHOLD)
	return results, nil
}

func searchStage1NamesOptimizedWithLanguageExtensions(client *qdrant.Client, ctx context.Context, config QdrantConfig, nameHash string, queryLangExt LanguageExtensions, topK uint64) ([]SearchResult, bool, error) {
	log.Printf("Stage 1 (Optimized): Names search for hash %s", nameHash)

	// Use channels for parallel exact and similarity searches
	type searchResult struct {
		results []SearchResult
		isExact bool
		err     error
	}

	resultChan := make(chan searchResult, 2)

	// Launch exact search in parallel
	go func() {
		exactResults, err := searchExactByPayload(client, ctx, config.CollectionName, "names", nameHash)
		resultChan <- searchResult{results: exactResults, isExact: true, err: err}
	}()

	// Launch similarity search in parallel with adaptive limit and language extension filtering
	go func() {
		adaptiveLimit := topK * 5 // Start with smaller limit for better performance
		if topK <= 3 {
			adaptiveLimit = topK * 10 // Increase for small topK
		}

		similarResults, err := performSimilaritySearchOptimizedWithLanguageExtensions(client, ctx, config.CollectionName, "names", nameHash, queryLangExt, adaptiveLimit, nil)
		resultChan <- searchResult{results: similarResults, isExact: false, err: err}
	}()

	// Collect results from both searches
	var exactResults, similarResults []SearchResult
	var exactErr, similarErr error
	hasExactMatch := false

	for i := 0; i < 2; i++ {
		result := <-resultChan
		if result.isExact {
			exactResults = result.results
			exactErr = result.err
			if len(exactResults) > 0 {
				hasExactMatch = true
			}
		} else {
			similarResults = result.results
			similarErr = result.err
		}
	}

	// Handle errors
	if exactErr != nil {
		log.Printf("Stage 1: Exact search failed: %v", exactErr)
	}
	if similarErr != nil {
		log.Printf("Stage 1: Similarity search failed: %v", similarErr)
		if len(exactResults) > 0 {
			return exactResults, hasExactMatch, nil
		}
		return nil, false, similarErr
	}

	// Combine exact and similar results
	allResults := append(exactResults, similarResults...)

	// Apply adaptive threshold with performance-aware limits
	threshold := calculateAdaptiveThresholdOptimized(allResults, "Stage 1 Names", hasExactMatch)

	// Filter by adaptive threshold
	var filteredResults []SearchResult
	for _, result := range allResults {
		if result.Distance <= threshold {
			filteredResults = append(filteredResults, result)
		}
	}

	// Limit results for better stage 2 performance
	maxCandidates := int(topK * 3) // Cap candidates to improve performance
	if len(filteredResults) > maxCandidates {
		// Sort by distance and take best candidates
		sort.Slice(filteredResults, func(i, j int) bool {
			return filteredResults[i].Distance < filteredResults[j].Distance
		})
		filteredResults = filteredResults[:maxCandidates]
	}

	log.Printf("Stage 1 (Optimized): Applied threshold %.1f, kept %d/%d candidates",
		threshold, len(filteredResults), len(allResults))

	return filteredResults, hasExactMatch, nil
}

func performSimilaritySearchOptimizedWithLanguageExtensions(client *qdrant.Client, ctx context.Context, collectionName, vectorName, hash string, queryLangExt LanguageExtensions, topK uint64, excludeResults []SearchResult) ([]SearchResult, error) {
	queryVector, err := HexSimhashToVector(hash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	// Build optimized filter with language extension filtering
	filter := buildOptimizedFilterWithLanguageExtensions(vectorName, queryLangExt, excludeResults)

	// Perform search with aggressive optimization parameters
	queryReq := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          &vectorName,
		Limit:          qdrant.PtrOf(topK),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error performing optimized search: %v", err)
	}

	// Convert results with quality filtering and language extension scoring
	var results []SearchResult
	for _, point := range searchResp {
		result := convertPointToResult(point)

		results = append(results, result)
	}

	return results, nil
}

func buildOptimizedFilterWithLanguageExtensions(vectorName string, queryLangExt LanguageExtensions, excludeResults []SearchResult) *qdrant.Filter {
	var mustNotConditions []*qdrant.Condition

	// Exclude already found results (limit to avoid huge filters)
	if len(excludeResults) > 0 {
		maxExclusions := 100 // Limit exclusions for performance
		for i, excludeResult := range excludeResults {
			if i >= maxExclusions {
				break
			}
			mustNotConditions = append(mustNotConditions, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: []*qdrant.PointId{qdrant.NewIDNum(excludeResult.ID)},
					},
				},
			})
		}
	}

	// Build filter with must conditions for language extensions (if provided)
	var mustConditions []*qdrant.Condition
	if len(queryLangExt) > 0 && vectorName == "names" {
		// Only apply language extension filtering to names search for better matching
		langExtConditions := buildLanguageExtensionFilter(queryLangExt, 30.0) // 30% tolerance
		if len(langExtConditions) > 0 {
			// At least one language should match within tolerance
			mustConditions = append(mustConditions, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Filter{
					Filter: &qdrant.Filter{
						Should: langExtConditions,
					},
				},
			})
		}
	}

	filter := &qdrant.Filter{
		MustNot: mustNotConditions,
	}

	if len(mustConditions) > 0 {
		filter.Must = mustConditions
	}

	return filter
}

// searchStage2DirsOptimized performs optimized directory filtering
func searchStage2DirsOptimized(client *qdrant.Client, ctx context.Context, config QdrantConfig, dirHash string, stage1Candidates []SearchResult) ([]SearchResult, error) {
	log.Printf("Stage 2 (Optimized): Directory filtering for hash %s on %d candidates", dirHash, len(stage1Candidates))

	if len(stage1Candidates) == 0 {
		return stage1Candidates, nil
	}

	// For small candidate sets, be more permissive
	if len(stage1Candidates) <= 5 {
		log.Printf("Stage 2: Small candidate set, skipping dir filtering for performance")
		return stage1Candidates, nil
	}

	// Build candidate ID filter (batch efficiently)
	candidateIDs := make([]*qdrant.PointId, len(stage1Candidates))
	for i, candidate := range stage1Candidates {
		candidateIDs[i] = qdrant.NewIDNum(candidate.ID)
	}

	// Convert dir hash to vector
	queryVector, err := HexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting dir hash to vector: %v", err)
	}

	// Optimized filter for candidates only
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: candidateIDs,
					},
				},
			},
		},
	}

	// Optimized search with score threshold
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          qdrant.PtrOf("dirs"),
		Limit:          qdrant.PtrOf(uint64(len(stage1Candidates))),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in optimized stage 2 dir search: %v", err)
	}

	// Process results with optimized scoring
	stage1Map := make(map[uint64]SearchResult)
	for _, candidate := range stage1Candidates {
		stage1Map[candidate.ID] = candidate
	}

	var filteredResults []SearchResult
	for _, point := range searchResp {
		dirResult := convertPointToResult(point)

		if stage1Result, exists := stage1Map[dirResult.ID]; exists {
			// Optimized scoring: 70% names, 30% dirs for better performance
			combinedScore := (stage1Result.Distance * 0.7) + (dirResult.Distance * 0.3)
			dirResult.Distance = combinedScore
			filteredResults = append(filteredResults, dirResult)
		}
	}

	// Sort by combined score
	sort.Slice(filteredResults, func(i, j int) bool {
		return filteredResults[i].Distance < filteredResults[j].Distance
	})

	log.Printf("Stage 2 (Optimized): Kept %d/%d candidates after dir filtering",
		len(filteredResults), len(stage1Candidates))

	return filteredResults, nil
}

// searchStage3ContentsOptimized performs optimized final content tie-breaking
func searchStage3ContentsOptimized(client *qdrant.Client, ctx context.Context, config QdrantConfig, contentHash string, stage2Candidates []SearchResult, topK uint64) ([]SearchResult, error) {
	log.Printf("Stage 3 (Optimized): Content tie-breaking for hash %s on %d candidates", contentHash, len(stage2Candidates))

	if len(stage2Candidates) == 0 {
		return stage2Candidates, nil
	}

	// For very small sets or if we already have topK, skip content filtering for performance
	if len(stage2Candidates) <= int(topK) {
		log.Printf("Stage 3: Candidate count <= topK, returning stage 2 results")
		if len(stage2Candidates) > int(topK) {
			return stage2Candidates[:topK], nil
		}
		return stage2Candidates, nil
	}

	// Build candidate ID filter
	candidateIDs := make([]*qdrant.PointId, len(stage2Candidates))
	for i, candidate := range stage2Candidates {
		candidateIDs[i] = qdrant.NewIDNum(candidate.ID)
	}

	// Convert content hash to vector
	queryVector, err := HexSimhashToVector(contentHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting content hash to vector: %v", err)
	}

	// Optimized filter for final candidates
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_HasId{
					HasId: &qdrant.HasIdCondition{
						HasId: candidateIDs,
					},
				},
			},
		},
	}

	// Final optimized search
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          qdrant.PtrOf("contents"),
		Limit:          qdrant.PtrOf(topK * 2), // Get a bit more for better ranking
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in optimized stage 3 content search: %v", err)
	}

	// Final scoring and ranking
	stage2Map := make(map[uint64]SearchResult)
	for _, candidate := range stage2Candidates {
		stage2Map[candidate.ID] = candidate
	}

	var finalResults []SearchResult
	for _, point := range searchResp {
		contentResult := convertPointToResult(point)

		if stage2Result, exists := stage2Map[contentResult.ID]; exists {
			// Final optimized scoring: 50% previous stages, 50% content
			finalScore := (stage2Result.Distance * 0.5) + (contentResult.Distance * 0.5)
			contentResult.Distance = finalScore
			finalResults = append(finalResults, contentResult)
		}
	}

	// Sort by final combined score
	sort.Slice(finalResults, func(i, j int) bool {
		return finalResults[i].Distance < finalResults[j].Distance
	})

	// Return top K results
	if len(finalResults) > int(topK) {
		finalResults = finalResults[:topK]
	}

	log.Printf("Stage 3 (Optimized): Final ranking complete, returning top %d results", len(finalResults))

	return finalResults, nil
}

func calculateAdaptiveThresholdOptimized(results []SearchResult, stageName string, hasExactMatch bool) float32 {
	if len(results) == 0 {
		return HIGH_SIMILARITY_THRESHOLD
	}

	// Sort by distance to analyze distribution
	sortedResults := make([]SearchResult, len(results))
	copy(sortedResults, results)
	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].Distance < sortedResults[j].Distance
	})

	// Calculate statistics
	minDist := sortedResults[0].Distance
	maxDist := sortedResults[len(sortedResults)-1].Distance

	// For performance, use simpler threshold calculation
	var threshold float32

	if hasExactMatch {
		// Be very selective when we have exact matches
		threshold = minDist + 1 // Very tight threshold
	} else if minDist <= HIGH_SIMILARITY_THRESHOLD {
		// Good quality results, moderate threshold
		threshold = minDist + 4
	} else {
		// Lower quality, be more permissive but reasonable
		threshold = float32(math.Min(float64(minDist+8), float64(MEDIUM_SIMILARITY_THRESHOLD)))
	}

	log.Printf("%s optimized threshold: min=%.1f, max=%.1f → threshold=%.1f (exact: %v)", stageName, minDist, maxDist, threshold, hasExactMatch)

	return threshold
}

// groupByComponent groups results by component name
func groupByComponent(results []SearchResult) []ComponentGroup {
	if len(results) == 0 {
		return []ComponentGroup{}
	}

	groups := make(map[string]*ComponentGroup)

	for _, result := range results {
		key := result.Component
		if key == "" {
			key = "unknown"
		}

		versionResult := VersionResult{
			Version:            result.Version,
			Distance:           result.Distance,
			URL:                result.URL,
			LanguageExtensions: result.LanguageExtensions,
			Metadata:           result.Metadata,
		}

		if group, exists := groups[key]; exists {
			// Add version to existing component group
			group.AllVersions = append(group.AllVersions, versionResult)
			group.ResultCount++

			// Update best match if this version has a better score
			if versionResult.Distance < group.BestMatch.Distance {
				group.BestMatch = versionResult
			}
		} else {
			// Create new component group
			groups[key] = &ComponentGroup{
				Component:   result.Component,
				Vendor:      result.Vendor,
				BestMatch:   versionResult,
				AllVersions: []VersionResult{versionResult},
				ResultCount: 1,
			}
		}
	}

	// Convert to slice and finalize
	var groupSlice []ComponentGroup
	for _, group := range groups {
		// Sort versions within group by distance (lower distance is better)
		sort.Slice(group.AllVersions, func(i, j int) bool {
			return group.AllVersions[i].Distance < group.AllVersions[j].Distance
		})

		// Create other versions list (excluding best match)
		for _, version := range group.AllVersions {
			if version.Version != group.BestMatch.Version || version.Distance != group.BestMatch.Distance {
				group.OtherVersions = append(group.OtherVersions, version.Version)
			}
		}

		groupSlice = append(groupSlice, *group)
	}

	// Sort groups by best match distance (lower distance is better)
	sort.Slice(groupSlice, func(i, j int) bool {
		return groupSlice[i].BestMatch.Distance < groupSlice[j].BestMatch.Distance
	})

	return groupSlice
}

// convertPointToResult converts a Qdrant ScoredPoint to SearchResult
func convertPointToResult(point *qdrant.ScoredPoint) SearchResult {
	result := SearchResult{
		Distance: point.Score,
		ID:       point.Id.GetNum(),
		Metadata: make(map[string]interface{}),
	}

	// Extract payload fields if they exist
	if point.Payload != nil {
		if val, exists := point.Payload["vendor"]; exists {
			result.Vendor = val.GetStringValue()
		}
		if val, exists := point.Payload["component"]; exists {
			result.Component = val.GetStringValue()
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
				langExt := make(LanguageExtensions)
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
				if langExt, err := parseLanguageExtensions(langExtStr); err == nil {
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

// SearchMultiStageWithLanguageExtensions performs optimized adaptive multi-stage search with language extension filtering
func SearchMultiStageWithLanguageExtensions(config QdrantConfig, dirHash, nameHash, contentHash string, queryLangExt LanguageExtensions, topK uint64) ([]ComponentGroup, error) {
	log.Printf("Starting optimized multi-stage search with adaptive thresholds")

	// Create a single shared client for all stages
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client")
	}
	defer client.Close()
	ctx := context.Background()

	// Stage 1: Optimized names search with early exact match detection and language extension filtering
	stage1Candidates, hasExactMatch, err := searchStage1NamesOptimizedWithLanguageExtensions(client, ctx, config, nameHash, queryLangExt, topK)
	if err != nil {
		return nil, fmt.Errorf("stage 1 (names) search failed: %v", err)
	}

	if len(stage1Candidates) == 0 {
		log.Printf("No candidates found in stage 1, returning empty results")
		return []ComponentGroup{}, nil
	}

	log.Printf("Stage 1 (names): Found %d candidates (exact match: %v)", len(stage1Candidates), hasExactMatch)

	// Early termination if we have exact match and it's the only result needed
	if hasExactMatch && topK == 1 && len(stage1Candidates) == 1 {
		log.Printf("Early termination: exact match found and only 1 result requested")
		componentGroups := groupByComponent(stage1Candidates)
		return componentGroups, nil
	}

	// Stage 2: Parallel dir filtering on candidates - structural validation
	stage2Candidates, err := searchStage2DirsOptimized(client, ctx, config, dirHash, stage1Candidates)
	if err != nil {
		log.Printf("Warning: Stage 2 (dirs) failed, using stage 1 results: %v", err)
		stage2Candidates = stage1Candidates
	}

	log.Printf("Stage 2 (dirs): Filtered to %d candidates", len(stage2Candidates))

	// Stage 3: Content tie-breaking - final ranking
	finalResults, err := searchStage3ContentsOptimized(client, ctx, config, contentHash, stage2Candidates, topK)
	if err != nil {
		log.Printf("Warning: Stage 3 (contents) failed, using stage 2 results: %v", err)
		finalResults = stage2Candidates
		if len(finalResults) > int(topK) {
			finalResults = finalResults[:topK]
		}
	}

	log.Printf("Stage 3 (contents): Final %d results", len(finalResults))

	// Group by component
	componentGroups := groupByComponent(finalResults)

	log.Printf("Optimized multi-stage search completed: %d results grouped into %d components", len(finalResults), len(componentGroups))
	return componentGroups, nil
}

// combineResults combines results from multiple searches and removes duplicates
// For hamming distance similarity, lower scores are better (we use manhattan distance in qdrant)
func combineResults(dirResults, nameResults, contentResults []SearchResult) []SearchResult {
	resultMap := make(map[uint64]SearchResult)

	// Add all results, keeping the best score for each ID
	// For manhattan distance, LOWER scores are better
	for _, result := range dirResults {
		if existing, exists := resultMap[result.ID]; !exists || result.Distance < existing.Distance {
			resultMap[result.ID] = result
		}
	}

	for _, result := range nameResults {
		if existing, exists := resultMap[result.ID]; !exists || result.Distance < existing.Distance {
			resultMap[result.ID] = result
		}
	}

	for _, result := range contentResults {
		if existing, exists := resultMap[result.ID]; !exists || result.Distance < existing.Distance {
			resultMap[result.ID] = result
		}
	}

	// Convert map to slice
	var results []SearchResult
	for _, result := range resultMap {
		results = append(results, result)
	}

	// Sort by distance (lower is better for hamming distance)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results
}

// parseLanguageExtensions parses the stringified JSON language extensions
func parseLanguageExtensions(langExtStr string) (LanguageExtensions, error) {
	var langExt LanguageExtensions
	if err := json.Unmarshal([]byte(langExtStr), &langExt); err != nil {
		log.Printf("Warning: failed to parse language_extensions '%s': %v", langExtStr, err)
		return nil, err
	}
	return langExt, nil
}

// calculateLanguageExtensionSimilarity calculates similarity score between two language extension maps
// Returns a score from 0.0 (no similarity) to 1.0 (perfect match)
func calculateLanguageExtensionSimilarity(query, candidate LanguageExtensions) float32 {
	if len(query) == 0 && len(candidate) == 0 {
		return 1.0 // Both empty, perfect match
	}
	if len(query) == 0 || len(candidate) == 0 {
		return 0.0 // One empty, no similarity
	}

	// Get all unique languages
	allLangs := make(map[string]bool)
	for lang := range query {
		allLangs[lang] = true
	}
	for lang := range candidate {
		allLangs[lang] = true
	}

	var similarity float64
	var totalWeight float64

	for lang := range allLangs {
		queryCount := float64(query[lang])
		candidateCount := float64(candidate[lang])

		// Weight by the maximum count to give more importance to languages with more files
		weight := math.Max(queryCount, candidateCount)
		if weight == 0 {
			continue
		}

		// Calculate similarity for this language (1.0 - normalized difference)
		diff := math.Abs(queryCount - candidateCount)
		maxCount := math.Max(queryCount, candidateCount)
		langSimilarity := 1.0 - (diff / maxCount)

		similarity += langSimilarity * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0.0
	}

	return float32(similarity / totalWeight)
}

// buildLanguageExtensionFilter creates Qdrant filters for language extension similarity
func buildLanguageExtensionFilter(queryLangExt LanguageExtensions, tolerancePercent float32) []*qdrant.Condition {
	if len(queryLangExt) == 0 {
		return nil
	}

	var conditions []*qdrant.Condition

	// For each language in query, create range filters
	for lang, count := range queryLangExt {
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
					Key: "language_extensions." + lang,
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
