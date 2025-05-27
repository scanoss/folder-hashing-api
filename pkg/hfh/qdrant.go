package hfh

import (
	"context"
	"fmt"
	"log"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection

	// Cosine similarity thresholds for dense vectors
	EXACT_MATCH_THRESHOLD       = 0.99 // Very high similarity for exact matches
	HIGH_SIMILARITY_THRESHOLD   = 0.85 // Strong similarity
	MEDIUM_SIMILARITY_THRESHOLD = 0.70 // Moderate similarity
	LOW_SIMILARITY_THRESHOLD    = 0.50 // Minimum acceptable similarity
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

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Score     float32                `json:"score"`
	ID        uint64                 `json:"id"`
	Vendor    string                 `json:"vendor"`
	Component string                 `json:"component"`
	Version   string                 `json:"version"`
	URL       string                 `json:"url"`
	Metadata  map[string]interface{} `json:"metadata"`
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
	Version  string                 `json:"version"`
	Score    float32                `json:"score"`
	URL      string                 `json:"url,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// hashToDenseVector converts a single 64-bit hash into a 64-dimensional dense vector
// Uses -1.0 for unset bits and 1.0 for set bits, optimized for cosine similarity
func hashToDenseVector(hash uint64) []float32 {
	vector := make([]float32, 64)
	for i := 0; i < 64; i++ {
		if (hash>>i)&1 == 1 {
			vector[i] = 1.0 // Bit is set
		} else {
			vector[i] = -1.0 // Bit is unset
		}
	}
	return vector
}

// SearchByDirHash searches for similar projects using directory structure hash
func SearchByDirHash(config QdrantConfig, dirHash uint64, topK uint64) ([]SearchResult, error) {
	return searchByHash(config, "dirs", dirHash, topK)
}

// SearchByNameHash searches for similar projects using component names hash
func SearchByNameHash(config QdrantConfig, nameHash uint64, topK uint64) ([]SearchResult, error) {
	return searchByHash(config, "names", nameHash, topK)
}

// SearchByContentHash searches for similar projects using content hash
func SearchByContentHash(config QdrantConfig, contentHash uint64, topK uint64) ([]SearchResult, error) {
	return searchByHash(config, "contents", contentHash, topK)
}

// searchByHash performs a similarity search using the specified vector type with cosine similarity
func searchByHash(config QdrantConfig, vectorName string, hash uint64, topK uint64) ([]SearchResult, error) {
	// Create Qdrant client
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("error connecting to Qdrant: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Check if collection exists
	exists, err := client.CollectionExists(ctx, config.CollectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking collection existence: %v", err)
	}
	if !exists {
		return nil, fmt.Errorf("collection %s does not exist", config.CollectionName)
	}

	// Convert hash to dense vector
	queryVector := hashToDenseVector(hash)

	log.Printf("Searching collection %s using vector %s for hash %016x", config.CollectionName, vectorName, hash)

	// Perform the search with score threshold to filter low similarity results
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          &vectorName, // Specify which named vector to use
		Limit:          qdrant.PtrOf(topK),
		ScoreThreshold: qdrant.PtrOf(float32(LOW_SIMILARITY_THRESHOLD)), // Only return results above minimum threshold
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false), // Don't need vectors in response
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
		if result.Score >= HIGH_SIMILARITY_THRESHOLD {
			log.Printf("High similarity match: %s %s (score: %.3f)", result.Component, result.Version, result.Score)
		} else if result.Score >= MEDIUM_SIMILARITY_THRESHOLD {
			log.Printf("Medium similarity match: %s %s (score: %.3f)", result.Component, result.Version, result.Score)
		} else if result.Score >= LOW_SIMILARITY_THRESHOLD {
			log.Printf("Low similarity match: %s %s (score: %.3f)", result.Component, result.Version, result.Score)
		}

		results = append(results, result)
	}

	log.Printf("Found %d results with score >= %.2f", len(results), LOW_SIMILARITY_THRESHOLD)
	return results, nil
}

// SearchCombined performs searches across all three vector types and combines results
func SearchCombined(config QdrantConfig, dirHash, nameHash, contentHash uint64, topK uint64) ([]ComponentGroup, error) {
	// Search each vector type
	dirResults, err := SearchByDirHash(config, dirHash, topK)
	if err != nil {
		log.Printf("Warning: Dir hash search failed: %v", err)
		dirResults = []SearchResult{}
	}

	nameResults, err := SearchByNameHash(config, nameHash, topK)
	if err != nil {
		log.Printf("Warning: Name hash search failed: %v", err)
		nameResults = []SearchResult{}
	}

	contentResults, err := SearchByContentHash(config, contentHash, topK)
	if err != nil {
		log.Printf("Warning: Content hash search failed: %v", err)
		contentResults = []SearchResult{}
	}

	// Combine and deduplicate results
	allResults := combineResults(dirResults, nameResults, contentResults)

	// Group by component
	componentGroups := groupByComponent(allResults)

	log.Printf("Combined search found %d total results grouped into %d components", len(allResults), len(componentGroups))
	return componentGroups, nil
}

// combineResults combines results from multiple searches and removes duplicates
// For cosine similarity, higher scores are better
func combineResults(dirResults, nameResults, contentResults []SearchResult) []SearchResult {
	resultMap := make(map[uint64]SearchResult)

	// Add all results, keeping the best score for each ID
	for _, result := range dirResults {
		if existing, exists := resultMap[result.ID]; !exists || result.Score > existing.Score {
			resultMap[result.ID] = result
		}
	}

	for _, result := range nameResults {
		if existing, exists := resultMap[result.ID]; !exists || result.Score > existing.Score {
			resultMap[result.ID] = result
		}
	}

	for _, result := range contentResults {
		if existing, exists := resultMap[result.ID]; !exists || result.Score > existing.Score {
			resultMap[result.ID] = result
		}
	}

	// Convert map to slice and sort by score (descending - higher is better for cosine)
	var results []SearchResult
	for _, result := range resultMap {
		results = append(results, result)
	}

	// Sort by score (higher is better for cosine similarity)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
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
			Version:  result.Version,
			Score:    result.Score,
			URL:      result.URL,
			Metadata: result.Metadata,
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
		// Sort versions within group by score (descending)
		for i := 0; i < len(group.AllVersions); i++ {
			for j := i + 1; j < len(group.AllVersions); j++ {
				if group.AllVersions[i].Score < group.AllVersions[j].Score {
					group.AllVersions[i], group.AllVersions[j] = group.AllVersions[j], group.AllVersions[i]
				}
			}
		}

		// Create other versions list (excluding best match)
		for _, version := range group.AllVersions {
			if version.Version != group.BestMatch.Version || version.Score != group.BestMatch.Score {
				group.OtherVersions = append(group.OtherVersions, version.Version)
			}
		}

		groupSlice = append(groupSlice, *group)
	}

	// Sort groups by best match score (descending)
	for i := 0; i < len(groupSlice); i++ {
		for j := i + 1; j < len(groupSlice); j++ {
			if groupSlice[i].BestMatch.Score < groupSlice[j].BestMatch.Score {
				groupSlice[i], groupSlice[j] = groupSlice[j], groupSlice[i]
			}
		}
	}

	return groupSlice
}

// convertPointToResult converts a Qdrant ScoredPoint to SearchResult
func convertPointToResult(point *qdrant.ScoredPoint) SearchResult {
	result := SearchResult{
		Score:    point.Score,
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

// SearchExact searches for exact hash matches using cosine similarity
func SearchExact(config QdrantConfig, dirHash, nameHash, contentHash uint64) (*SearchResult, error) {
	// Try each vector type to find exact matches
	vectorTypes := []struct {
		name string
		hash uint64
	}{
		{"contents", contentHash}, // Try content first (most specific)
		{"dirs", dirHash},
		{"names", nameHash},
	}

	for _, vt := range vectorTypes {
		results, err := searchByHash(config, vt.name, vt.hash, 1)
		if err != nil {
			continue
		}

		if len(results) > 0 && results[0].Score >= EXACT_MATCH_THRESHOLD {
			// Found exact match (score >= 0.99 for cosine similarity)
			log.Printf("Found exact match using %s vector: %s %s (score: %.3f)", vt.name, results[0].Component, results[0].Version, results[0].Score)
			return &results[0], nil
		}
	}

	return nil, fmt.Errorf("no exact match found")
}

// SearchProgressive performs progressive search with different similarity thresholds
func SearchProgressive(config QdrantConfig, dirHash, nameHash, contentHash uint64, topK uint64) ([]ComponentGroup, error) {
	log.Printf("Starting progressive search for hashes: dir=%016x, name=%016x, content=%016x", dirHash, nameHash, contentHash)

	// Stage 1: Look for exact or near-exact matches (>= 0.99)
	log.Println("Stage 1: Searching for exact matches...")
	exactResult, err := SearchExact(config, dirHash, nameHash, contentHash)
	if err == nil && exactResult != nil {
		log.Printf("Found exact match: %s %s", exactResult.Component, exactResult.Version)
		return []ComponentGroup{{
			Component:     exactResult.Component,
			Vendor:        exactResult.Vendor,
			BestMatch:     VersionResult{Version: exactResult.Version, Score: exactResult.Score, URL: exactResult.URL, Metadata: exactResult.Metadata},
			AllVersions:   []VersionResult{{Version: exactResult.Version, Score: exactResult.Score, URL: exactResult.URL, Metadata: exactResult.Metadata}},
			OtherVersions: []string{},
			ResultCount:   1,
		}}, nil
	}

	// Stage 2: Look for high similarity matches (>= 0.85)
	log.Println("Stage 2: Searching for high similarity matches...")
	results, err := SearchCombined(config, dirHash, nameHash, contentHash, topK)
	if err != nil {
		return nil, fmt.Errorf("combined search failed: %v", err)
	}

	// Filter for high-quality results
	var filteredGroups []ComponentGroup
	for _, group := range results {
		if group.BestMatch.Score >= HIGH_SIMILARITY_THRESHOLD {
			log.Printf("High similarity component found: %s (score: %.3f)", group.Component, group.BestMatch.Score)
			filteredGroups = append(filteredGroups, group)
		} else if group.BestMatch.Score >= MEDIUM_SIMILARITY_THRESHOLD {
			log.Printf("Medium similarity component found: %s (score: %.3f)", group.Component, group.BestMatch.Score)
			// Only include if we don't have enough high-quality results
			if len(filteredGroups) < 3 {
				filteredGroups = append(filteredGroups, group)
			}
		}
	}

	if len(filteredGroups) > 0 {
		log.Printf("Progressive search completed. Found %d high-quality component groups", len(filteredGroups))
		return filteredGroups, nil
	}

	// Stage 3: Fallback to any reasonable matches (>= 0.50)
	log.Println("Stage 3: Fallback to any reasonable matches...")
	for _, group := range results {
		if group.BestMatch.Score >= LOW_SIMILARITY_THRESHOLD {
			log.Printf("Low similarity component found: %s (score: %.3f)", group.Component, group.BestMatch.Score)
			filteredGroups = append(filteredGroups, group)
		}
	}

	if len(filteredGroups) > 0 {
		log.Printf("Progressive search completed. Found %d reasonable component groups", len(filteredGroups))
		return filteredGroups, nil
	}

	return nil, fmt.Errorf("no similar projects found above minimum similarity threshold (%.2f)", LOW_SIMILARITY_THRESHOLD)
}
