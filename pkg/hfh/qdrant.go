package hfh

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection

	// Manhattan distance thresholds for binary vectors (lower scores = better matches)
	EXACT_MATCH_THRESHOLD       = 0  // Perfect match (identical vectors)
	HIGH_SIMILARITY_THRESHOLD   = 4  // 1-4 bit differences
	MEDIUM_SIMILARITY_THRESHOLD = 12 // 5-12 bit differences
	LOW_SIMILARITY_THRESHOLD    = 20 // 13-20 bit differences
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
	Distance  float32                `json:"distance"`
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
	Distance float32                `json:"distance"`
	URL      string                 `json:"url,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
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

// SearchByDirHash searches for similar projects using directory structure hash
func SearchByDirHash(config QdrantConfig, dirHash string, topK uint64) ([]SearchResult, error) {
	return searchByHash(config, "dirs", dirHash, topK)
}

// SearchByNameHash searches for similar projects using component names hash
func SearchByNameHash(config QdrantConfig, nameHash string, topK uint64) ([]SearchResult, error) {
	return searchByHash(config, "names", nameHash, topK)
}

// SearchByContentHash searches for similar projects using content hash
func SearchByContentHash(config QdrantConfig, contentHash string, topK uint64) ([]SearchResult, error) {
	return searchByHash(config, "contents", contentHash, topK)
}

// searchByHash performs a similarity search using the specified vector type with Manhattan distance
func searchByHash(config QdrantConfig, vectorName string, hash string, topK uint64) ([]SearchResult, error) {
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
	queryVector, err := HexSimhashToVector(hash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	log.Printf("Searching collection %s using vector %s for hash %016x", config.CollectionName, vectorName, hash)

	// Perform the search with score threshold to filter low similarity results
	// For Manhattan distance, we want results BELOW the threshold (lower scores = better matches)
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Using:          &vectorName,
		Limit:          qdrant.PtrOf(topK),
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

	log.Printf("Found %d results with distance <= %d", len(results), LOW_SIMILARITY_THRESHOLD)
	return results, nil
}

func searchWithPrefetch(config QdrantConfig, dirHash, nameHash, contentHash string, topK uint64) ([]SearchResult, error) {
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
	dirVector, err := HexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	nameVector, err := HexSimhashToVector(nameHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	contentVector, err := HexSimhashToVector(contentHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting hash to vector: %v", err)
	}

	log.Printf("Searching collection %s using vector dirs for hash %016x", config.CollectionName, dirHash)
	log.Printf("Searching collection %s using vector names for hash %016x", config.CollectionName, nameHash)
	log.Printf("Searching collection %s using vector contents for hash %016x", config.CollectionName, contentHash)

	// Perform the search with score threshold to filter low similarity results
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Prefetch: []*qdrant.PrefetchQuery{
			{
				Query: qdrant.NewQuery(dirVector...),
				Using: qdrant.PtrOf("dirs"),
			},
			{
				Query: qdrant.NewQuery(nameVector...),
				Using: qdrant.PtrOf("names"),
			},
			{
				Query: qdrant.NewQuery(contentVector...),
				Using: qdrant.PtrOf("contents"),
			},
		},
		Query:       qdrant.NewQueryFusion(qdrant.Fusion_RRF),
		Limit:       qdrant.PtrOf(topK),
		WithPayload: qdrant.NewWithPayload(true),
		WithVectors: qdrant.NewWithVectors(false),
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error performing search: %v", err)
	}

	// Convert results and filter by quality
	var results []SearchResult
	for _, point := range searchResp {
		result := convertPointToResult(point)

		// Additional quality filtering based on distance
		// For manhattan distance, lower distances are better
		if result.Distance <= HIGH_SIMILARITY_THRESHOLD {
			log.Printf("High similarity match: %s %s (distance: %.3f)", result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else if result.Distance <= MEDIUM_SIMILARITY_THRESHOLD {
			log.Printf("Medium similarity match: %s %s (distance: %.3f)", result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else if result.Distance <= LOW_SIMILARITY_THRESHOLD {
			log.Printf("Low similarity match: %s %s (distance: %.3f)", result.Component, result.Version, result.Distance)
			results = append(results, result)
		} else {
			// Distance is too high (poor match) for manhattan distance
			log.Printf("Poor match: %s %s (distance: %.3f)", result.Component, result.Version, result.Distance)
			continue
		}
	}

	log.Printf("Found %d results with score <= %d", len(results), MEDIUM_SIMILARITY_THRESHOLD)
	return results, nil
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
			Distance: result.Distance,
			URL:      result.URL,
			Metadata: result.Metadata,
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

// SearchExact searches for exact hash matches using manhattan distance
func SearchExact(config QdrantConfig, dirHash, nameHash, contentHash string) (*SearchResult, error) {
	// Try each vector type to find exact matches
	vectorTypes := []struct {
		name string
		hash string
	}{
		{"contents", contentHash},
		{"dirs", dirHash},
		{"names", nameHash},
	}

	for _, vt := range vectorTypes {
		results, err := searchByHash(config, vt.name, vt.hash, 1)
		if err != nil {
			continue
		}

		if len(results) > 0 && results[0].Distance <= EXACT_MATCH_THRESHOLD {
			// Found exact match (distance <= 0 for manhattan distance means identical vectors)
			log.Printf("Found exact match using %s vector: %s %s (distance: %.3f)", vt.name, results[0].Component, results[0].Version, results[0].Distance)
			return &results[0], nil
		}
	}

	return nil, fmt.Errorf("no exact match found")
}

func SearchWithPrefetch(config QdrantConfig, dirHash, nameHash, contentHash string, topK uint64) ([]ComponentGroup, error) {
	searchResults, err := searchWithPrefetch(config, dirHash, nameHash, contentHash, topK)
	if err != nil {
		log.Printf("Error searching in Qdrant: %v", err)
		return []ComponentGroup{}, nil
	}

	// Group by component
	componentGroups := groupByComponent(searchResults)

	log.Printf("Multi-stage search completed. Found %d results grouped into %d components", len(componentGroups), len(componentGroups))
	return componentGroups, nil
}

// SearchMultiStage performs searches across all three vector types and combines results
func SearchMultiStage(config QdrantConfig, dirHash, nameHash, contentHash string, topK uint64) ([]ComponentGroup, error) {
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
