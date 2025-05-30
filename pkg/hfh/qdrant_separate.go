package hfh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection

	// Collection names for separate collections approach
	DirsCollectionName     = "dirs_collection"
	NamesCollectionName    = "names_collection"
	ContentsCollectionName = "contents_collection"

	// Optimized thresholds for approximate search only
	HIGH_SIMILARITY_THRESHOLD_APPROX   = 5  // 1-5 bit differences (very strict)
	MEDIUM_SIMILARITY_THRESHOLD_APPROX = 12 // 6-12 bit differences (moderate)
	LOW_SIMILARITY_THRESHOLD_APPROX    = 20 // 13-20 bit differences (permissive)
)

// QdrantSeparateConfig holds configuration for separate collections approach
type QdrantSeparateConfig struct {
	Host string
	Port int
}

type LanguageExtensions map[string]int32

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Distance           float32            `json:"distance"`
	ID                 uint64             `json:"id"`
	Vendor             string             `json:"vendor"`
	Component          string             `json:"component"`
	Version            string             `json:"version"`
	URL                string             `json:"url"`
	LanguageExtensions LanguageExtensions `json:"language_extensions,omitempty"`
	Metadata           map[string]any     `json:"metadata"`
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
	Version            string             `json:"version"`
	Distance           float32            `json:"distance"`
	URL                string             `json:"url,omitempty"`
	LanguageExtensions LanguageExtensions `json:"language_extensions,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
}

// NewQdrantSeparateConfig creates a new config for separate collections
func NewQdrantSeparateConfig(host string, port int) QdrantSeparateConfig {
	return QdrantSeparateConfig{
		Host: host,
		Port: port,
	}
}

// performApproximateSearch performs only approximate vector similarity search
func performApproximateSearch(client *qdrant.Client, ctx context.Context, collectionName, hash string, topK uint64) ([]SearchResult, error) {
	return performApproximateSearchWithLanguageExtensions(client, ctx, collectionName, hash, nil, topK)
}

func HexSimhashToVector(hexHashString string, bits int) ([]float32, error) {
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
			vector[i] = 0.0 // Explicitly set to 0.0, though it's the default
		}
	}

	return vector, nil
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

// performApproximateSearchWithLanguageExtensions performs approximate search with optional language extension filtering
func performApproximateSearchWithLanguageExtensions(client *qdrant.Client, ctx context.Context, collectionName, hash string, queryLangExt LanguageExtensions, topK uint64) ([]SearchResult, error) {
	// Convert hash to dense vector
	queryVector, err := HexSimhashToVector(hash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	log.Printf("Performing approximate search in %s for hash %s", collectionName, hash)

	// Build filter for language extensions if provided
	var filter *qdrant.Filter
	if len(queryLangExt) > 0 && collectionName == NamesCollectionName {
		// Apply language extension filtering only to names collection for better matching
		langExtConditions := buildLanguageExtensionFilter(queryLangExt, 30.0) // 30% tolerance for separate collections
		if len(langExtConditions) > 0 {
			filter = &qdrant.Filter{
				Should: langExtConditions, // At least one language should match within tolerance
			}
		}
	}

	// Optimized search with aggressive performance parameters
	queryReq := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(topK),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			HnswEf:      qdrant.PtrOf(uint64(128)), // Moderate ef for balance of speed/quality
			Exact:       qdrant.PtrOf(false),       // Always approximate
			IndexedOnly: qdrant.PtrOf(true),        // Only search indexed data
		},
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error performing approximate search in %s: %v", collectionName, err)
	}

	// Convert results and apply quality filtering
	var results []SearchResult
	for _, point := range searchResp {
		result := convertPointToResult(point)

		// Apply quality thresholds
		if result.Distance <= HIGH_SIMILARITY_THRESHOLD_APPROX {
			log.Printf("High quality match in %s: %s %s (distance: %.3f)", collectionName, result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else if result.Distance <= MEDIUM_SIMILARITY_THRESHOLD_APPROX {
			log.Printf("Medium quality match in %s: %s %s (distance: %.3f)", collectionName, result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else if result.Distance <= LOW_SIMILARITY_THRESHOLD_APPROX {
			log.Printf("Lower quality match in %s: %s %s (distance: %.3f)", collectionName, result.Component, result.Version, result.Distance)
			results = append(results, result)
		}
		// Ignore matches with distance > LOW_SIMILARITY_THRESHOLD_APPROX
	}

	log.Printf("Found %d quality results in %s (distance <= %d)", len(results), collectionName, LOW_SIMILARITY_THRESHOLD_APPROX)
	return results, nil
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

// searchStage1NamesApproximate performs stage 1 with only approximate search
func searchStage1NamesApproximate(client *qdrant.Client, ctx context.Context, config QdrantSeparateConfig, nameHash string, topK uint64) ([]SearchResult, error) {
	return searchStage1NamesApproximateWithLanguageExtensions(client, ctx, config, nameHash, nil, topK)
}

// searchStage1NamesApproximateWithLanguageExtensions performs stage 1 with language extension filtering
func searchStage1NamesApproximateWithLanguageExtensions(client *qdrant.Client, ctx context.Context, config QdrantSeparateConfig, nameHash string, queryLangExt LanguageExtensions, topK uint64) ([]SearchResult, error) {
	log.Printf("Stage 1 (Approximate): Names search for hash %s with language extensions", nameHash)

	// Increase search limit for better candidate selection
	searchLimit := topK * 3
	if searchLimit < 50 {
		searchLimit = 50 // Minimum reasonable limit
	}

	results, err := performApproximateSearchWithLanguageExtensions(client, ctx, NamesCollectionName, nameHash, queryLangExt, searchLimit)
	if err != nil {
		return nil, fmt.Errorf("stage 1 names search failed: %v", err)
	}

	// Apply adaptive threshold based on result quality
	threshold := calculateAdaptiveThresholdApproximate(results, "Stage 1 Names")

	var filteredResults []SearchResult
	for _, result := range results {
		if result.Distance <= threshold {
			filteredResults = append(filteredResults, result)
		}
	}

	// Limit candidates for better stage 2 performance
	maxCandidates := int(topK * 2)
	if len(filteredResults) > maxCandidates {
		sort.Slice(filteredResults, func(i, j int) bool {
			return filteredResults[i].Distance < filteredResults[j].Distance
		})
		filteredResults = filteredResults[:maxCandidates]
	}

	log.Printf("Stage 1 (Approximate): Applied threshold %.1f, kept %d/%d candidates", threshold, len(filteredResults), len(results))
	return filteredResults, nil
}

// searchStage2DirsApproximate performs stage 2 directory filtering on candidates
func searchStage2DirsApproximate(client *qdrant.Client, ctx context.Context, config QdrantSeparateConfig, dirHash string, stage1Candidates []SearchResult) ([]SearchResult, error) {
	log.Printf("Stage 2 (Approximate): Directory filtering for hash %s on %d candidates", dirHash, len(stage1Candidates))

	if len(stage1Candidates) == 0 {
		return stage1Candidates, nil
	}

	// For small candidate sets, be more permissive
	if len(stage1Candidates) <= 3 {
		log.Printf("Stage 2: Very small candidate set, skipping dir filtering")
		return stage1Candidates, nil
	}

	// Build candidate ID filter for efficient lookup
	candidateIDs := make([]*qdrant.PointId, len(stage1Candidates))
	for i, candidate := range stage1Candidates {
		candidateIDs[i] = qdrant.NewIDNum(candidate.ID)
	}

	// Convert dir hash to vector
	queryVector, err := HexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting dir hash to vector: %v", err)
	}

	// Search only within candidate IDs
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

	queryReq := &qdrant.QueryPoints{
		CollectionName: DirsCollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(uint64(len(stage1Candidates))),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			HnswEf:      qdrant.PtrOf(uint64(128)),
			Exact:       qdrant.PtrOf(false),
			IndexedOnly: qdrant.PtrOf(true),
		},
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in stage 2 dir search: %v", err)
	}

	// Combine scores from stage 1 and stage 2
	stage1Map := make(map[uint64]SearchResult)
	for _, candidate := range stage1Candidates {
		stage1Map[candidate.ID] = candidate
	}

	var filteredResults []SearchResult
	for _, point := range searchResp {
		dirResult := convertPointToResult(point)

		if stage1Result, exists := stage1Map[dirResult.ID]; exists {
			// Combined scoring: 70% names, 30% dirs
			combinedScore := (stage1Result.Distance * 0.7) + (dirResult.Distance * 0.3)
			dirResult.Distance = combinedScore
			filteredResults = append(filteredResults, dirResult)
		}
	}

	// Sort by combined score
	sort.Slice(filteredResults, func(i, j int) bool {
		return filteredResults[i].Distance < filteredResults[j].Distance
	})

	log.Printf("Stage 2 (Approximate): Kept %d/%d candidates after dir filtering", len(filteredResults), len(stage1Candidates))
	return filteredResults, nil
}

// searchStage3ContentsApproximate performs stage 3 final content ranking
func searchStage3ContentsApproximate(client *qdrant.Client, ctx context.Context, config QdrantSeparateConfig, contentHash string, stage2Candidates []SearchResult, topK uint64) ([]SearchResult, error) {
	log.Printf("Stage 3 (Approximate): Content ranking for hash %s on %d candidates", contentHash, len(stage2Candidates))

	if len(stage2Candidates) == 0 {
		return stage2Candidates, nil
	}

	// For very small sets, return directly
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

	queryReq := &qdrant.QueryPoints{
		CollectionName: ContentsCollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(topK * 2), // Get more for better ranking
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			HnswEf:      qdrant.PtrOf(uint64(128)),
			Exact:       qdrant.PtrOf(false),
			IndexedOnly: qdrant.PtrOf(true),
		},
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in stage 3 content search: %v", err)
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
			// Final scoring: 60% previous stages, 40% content
			finalScore := (stage2Result.Distance * 0.6) + (contentResult.Distance * 0.4)
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

	log.Printf("Stage 3 (Approximate): Final ranking complete, returning top %d results", len(finalResults))
	return finalResults, nil
}

// calculateAdaptiveThresholdApproximate calculates adaptive threshold for approximate search results
func calculateAdaptiveThresholdApproximate(results []SearchResult, stageName string) float32 {
	if len(results) == 0 {
		return HIGH_SIMILARITY_THRESHOLD_APPROX
	}

	// Sort by distance to analyze distribution
	sortedResults := make([]SearchResult, len(results))
	copy(sortedResults, results)
	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].Distance < sortedResults[j].Distance
	})

	minDist := sortedResults[0].Distance

	var threshold float32
	if minDist <= HIGH_SIMILARITY_THRESHOLD_APPROX {
		// Excellent quality results, be selective
		threshold = minDist + 3
	} else if minDist <= MEDIUM_SIMILARITY_THRESHOLD_APPROX {
		// Good quality results, moderate threshold
		threshold = minDist + 5
	} else {
		// Lower quality, be more permissive but reasonable
		threshold = float32(math.Min(float64(minDist+8), float64(LOW_SIMILARITY_THRESHOLD_APPROX)))
	}

	log.Printf("%s approximate threshold: min=%.1f → threshold=%.1f", stageName, minDist, threshold)
	return threshold
}

// SearchMultiStageApproximate performs optimized multi-stage search with separate collections and approximate search only
func SearchMultiStageApproximate(config QdrantSeparateConfig, dirHash, nameHash, contentHash string, topK uint64) ([]ComponentGroup, error) {
	return SearchMultiStageApproximateWithLanguageExtensions(config, dirHash, nameHash, contentHash, nil, topK)
}

// SearchMultiStageApproximateWithLanguageExtensions performs optimized multi-stage search with language extension filtering
func SearchMultiStageApproximateWithLanguageExtensions(config QdrantSeparateConfig, dirHash, nameHash, contentHash string, queryLangExt LanguageExtensions, topK uint64) ([]ComponentGroup, error) {
	log.Printf("Starting optimized multi-stage approximate search with separate collections")

	// Create a single shared client
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %v", err)
	}
	defer client.Close()

	// Set context with shorter timeout for faster queries
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Stage 1: Names search with approximate search only and language extension filtering
	stage1Candidates, err := searchStage1NamesApproximateWithLanguageExtensions(client, ctx, config, nameHash, queryLangExt, topK)
	if err != nil {
		return nil, fmt.Errorf("stage 1 (names) search failed: %v", err)
	}

	if len(stage1Candidates) == 0 {
		log.Printf("No candidates found in stage 1, returning empty results")
		return []ComponentGroup{}, nil
	}

	log.Printf("Stage 1 (names): Found %d candidates", len(stage1Candidates))

	// Stage 2: Directory filtering on candidates
	stage2Candidates, err := searchStage2DirsApproximate(client, ctx, config, dirHash, stage1Candidates)
	if err != nil {
		log.Printf("Warning: Stage 2 (dirs) failed, using stage 1 results: %v", err)
		stage2Candidates = stage1Candidates
	}

	log.Printf("Stage 2 (dirs): Filtered to %d candidates", len(stage2Candidates))

	// Stage 3: Content ranking
	finalResults, err := searchStage3ContentsApproximate(client, ctx, config, contentHash, stage2Candidates, topK)
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

	log.Printf("Multi-stage approximate search completed: %d results grouped into %d components", len(finalResults), len(componentGroups))
	return componentGroups, nil
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
