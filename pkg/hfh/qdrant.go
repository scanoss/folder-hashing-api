package hfh

import (
	"context"
	"fmt"
	"log"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection
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

// hashToVector converts a single 64-bit hash into a 64-dimensional binary vector
func hashToVector(hash uint64) []float32 {
	vector := make([]float32, 64)
	for i := 0; i < 64; i++ {
		if (hash>>i)&1 == 1 {
			vector[i] = 1.0
		} else {
			vector[i] = 0.0
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

// searchByHash performs a similarity search using the specified vector type
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

	// Convert hash to vector
	queryVector := hashToVector(hash)

	log.Printf("Searching collection %s using vector %s for hash %016x", config.CollectionName, vectorName, hash)

	// Perform the search
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          &vectorName, // Specify which named vector to use
		Limit:          qdrant.PtrOf(topK),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false), // Don't need vectors in response
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error performing search: %v", err)
	}

	// Convert results
	var results []SearchResult
	for _, point := range searchResp {
		result := convertPointToResult(point)
		results = append(results, result)
	}

	log.Printf("Found %d results", len(results))
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

	// Convert map to slice and sort by score (descending)
	var results []SearchResult
	for _, result := range resultMap {
		results = append(results, result)
	}

	// Sort by score (higher is better)
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

// SearchExact searches for exact hash matches (score should be 0 for identical hashes)
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

		if len(results) > 0 && results[0].Score == 0 {
			// Found exact match (score 0 means identical vectors)
			log.Printf("Found exact match using %s vector: %s %s", vt.name, results[0].Component, results[0].Version)
			return &results[0], nil
		}
	}

	return nil, fmt.Errorf("no exact match found")
}
