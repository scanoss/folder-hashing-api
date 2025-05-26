package hfh

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 192 // Three 64-bit hashes concatenated (3 * 64 = 192)
)

// QdrantConfig holds Qdrant connection configuration
type QdrantConfig struct {
	Host           string
	Port           int
	CollectionName string
}

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Score        float32
	ID           uint64
	CombinedHash string
	Vendor       string
	Component    string
	Version      string
	URL          string
	Metadata     map[string]interface{}
	SearchStage  string // Which stage found this result
	HammingDist  int    // Hamming distance for this result
}

// SearchSimilarProjects searches for similar projects using multi-stage Hamming distance approach
func SearchSimilarProjects(config QdrantConfig, dirHash, nameHash, contentHash uint64, topK uint64) ([]SearchResult, error) {
	// Create Qdrant client
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("error connecting to Qdrant: %v", err)
	}
	defer client.Close()

	// Check if collection exists
	ctx := context.Background()
	exists, err := client.CollectionExists(ctx, config.CollectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking collection existence: %v", err)
	}
	if !exists {
		return nil, fmt.Errorf("collection '%s' does not exist", config.CollectionName)
	}

	// Convert hashes to hex strings for exact matching
	dirHashStr := fmt.Sprintf("%016x", dirHash)
	nameHashStr := fmt.Sprintf("%016x", nameHash)
	contentHashStr := fmt.Sprintf("%016x", contentHash)

	var allResults []SearchResult

	// Stage 1: Exact hash matching
	fmt.Println("Stage 1: Searching for exact hash matches...")
	exactResults, err := searchExactMatches(ctx, client, config.CollectionName, dirHashStr, nameHashStr, contentHashStr, topK)
	if err != nil {
		fmt.Printf("Warning: Stage 1 search failed: %v\n", err)
	} else if len(exactResults) > 0 {
		fmt.Printf("Stage 1: Found %d exact matches\n", len(exactResults))
		allResults = append(allResults, exactResults...)
		// If we have exact matches, return them immediately
		if len(allResults) >= int(topK) {
			return allResults[:topK], nil
		}
	} else {
		fmt.Println("Stage 1: No exact matches found")
	}

	// Stage 2: Component-aware similarity search
	fmt.Println("Stage 2: Searching with component awareness...")
	componentResults, err := searchComponentAware(ctx, client, config.CollectionName, dirHash, nameHash, contentHash, topK-uint64(len(allResults)))
	if err != nil {
		fmt.Printf("Warning: Stage 2 search failed: %v\n", err)
	} else if len(componentResults) > 0 {
		fmt.Printf("Stage 2: Found %d component-aware matches\n", len(componentResults))
		allResults = append(allResults, componentResults...)
		if len(allResults) >= int(topK) {
			return allResults[:topK], nil
		}
	} else {
		fmt.Println("Stage 2: No component-aware matches found")
	}

	// Stage 3: General similarity search
	fmt.Println("Stage 3: General similarity search...")
	generalResults, err := searchGeneralSimilarity(ctx, client, config.CollectionName, dirHash, nameHash, contentHash, topK-uint64(len(allResults)))
	if err != nil {
		fmt.Printf("Warning: Stage 3 search failed: %v\n", err)
	} else if len(generalResults) > 0 {
		fmt.Printf("Stage 3: Found %d general similarity matches\n", len(generalResults))
		allResults = append(allResults, generalResults...)
	} else {
		fmt.Println("Stage 3: No general similarity matches found")
	}

	// Ensure we don't exceed topK
	if len(allResults) > int(topK) {
		allResults = allResults[:topK]
	}

	return allResults, nil
}

// searchExactMatches performs exact hash matching using filters
func searchExactMatches(ctx context.Context, client *qdrant.Client, collectionName, dirHash, nameHash, contentHash string, limit uint64) ([]SearchResult, error) {
	// Create filter for exact hash matches
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: "hfh_dirs_hash",
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Text{Text: dirHash},
						},
					},
				},
			},
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: "hfh_names_hash",
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Text{Text: nameHash},
						},
					},
				},
			},
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: "hfh_contents_hash",
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Text{Text: contentHash},
						},
					},
				},
			},
		},
	}

	// Scroll through results with filter
	scrollResp, err := client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collectionName,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	})
	if err != nil {
		return nil, fmt.Errorf("error in exact match scroll: %v", err)
	}

	var results []SearchResult
	for _, point := range scrollResp {
		result := convertRetrievedPointToResult(point, "Stage 1: Exact Match", 0) // 0 Hamming distance for exact matches
		results = append(results, result)
	}

	return results, nil
}

// searchComponentAware performs component-aware similarity search
func searchComponentAware(ctx context.Context, client *qdrant.Client, collectionName string, dirHash, nameHash, contentHash uint64, limit uint64) ([]SearchResult, error) {
	// Convert three hashes to concatenated vector
	queryVector := HashesToVector(dirHash, nameHash, contentHash)

	// For component-aware search, we'll use a tighter similarity threshold
	// and potentially add component filters if we can extract component name from the query

	queryReq := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(uint64(limit * 2)), // Get more results to filter and re-rank
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(true), // Need vectors to calculate Hamming distance
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in component-aware search: %v", err)
	}

	var results []SearchResult
	queryVectorBinary := HashesToVector(dirHash, nameHash, contentHash)

	for _, point := range searchResp {
		// Calculate Hamming distance
		pointVector := point.Vectors.GetVector().GetData()
		hammingDist := calculateHammingDistance(queryVectorBinary, pointVector)

		// Apply stricter Hamming distance threshold for component-aware search
		if hammingDist <= 15 { // Allow up to 15 bits difference out of 192
			result := convertScoredPointToResult(point, "Stage 2: Component-Aware", hammingDist)
			results = append(results, result)
		}
	}

	// Sort by Hamming distance (lower is better)
	sortResultsByHammingDistance(results)

	// Return up to the requested limit
	if len(results) > int(limit) {
		results = results[:limit]
	}

	return results, nil
}

// searchGeneralSimilarity performs general similarity search as fallback
func searchGeneralSimilarity(ctx context.Context, client *qdrant.Client, collectionName string, dirHash, nameHash, contentHash uint64, limit uint64) ([]SearchResult, error) {
	// Convert three hashes to concatenated vector
	queryVector := HashesToVector(dirHash, nameHash, contentHash)

	queryReq := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(uint64(limit * 2)), // Get more results to filter and re-rank
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(true), // Need vectors to calculate Hamming distance
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in general similarity search: %v", err)
	}

	var results []SearchResult
	queryVectorBinary := HashesToVector(dirHash, nameHash, contentHash)

	for _, point := range searchResp {
		// Calculate Hamming distance
		pointVector := point.Vectors.GetVector().GetData()
		hammingDist := calculateHammingDistance(queryVectorBinary, pointVector)

		// Apply looser Hamming distance threshold for general search
		if hammingDist <= 30 { // Allow up to 30 bits difference out of 192
			result := convertScoredPointToResult(point, "Stage 3: General Similarity", hammingDist)
			results = append(results, result)
		}
	}

	// Sort by Hamming distance (lower is better)
	sortResultsByHammingDistance(results)

	// Return up to the requested limit
	if len(results) > int(limit) {
		results = results[:limit]
	}

	return results, nil
}

// convertRetrievedPointToResult converts a Qdrant RetrievedPoint to SearchResult
func convertRetrievedPointToResult(point *qdrant.RetrievedPoint, stage string, hammingDist int) SearchResult {
	result := SearchResult{
		Score:       0.0, // RetrievedPoint doesn't have a score
		ID:          point.Id.GetNum(),
		SearchStage: stage,
		HammingDist: hammingDist,
		Metadata:    make(map[string]interface{}),
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

// convertScoredPointToResult converts a Qdrant ScoredPoint to SearchResult
func convertScoredPointToResult(point *qdrant.ScoredPoint, stage string, hammingDist int) SearchResult {
	result := SearchResult{
		Score:       point.Score,
		ID:          point.Id.GetNum(),
		SearchStage: stage,
		HammingDist: hammingDist,
		Metadata:    make(map[string]interface{}),
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

// calculateHammingDistance calculates the Hamming distance between two binary vectors
func calculateHammingDistance(vec1, vec2 []float32) int {
	if len(vec1) != len(vec2) {
		return len(vec1) // Return maximum possible distance if lengths don't match
	}

	distance := 0
	for i := 0; i < len(vec1); i++ {
		if vec1[i] != vec2[i] {
			distance++
		}
	}
	return distance
}

// sortResultsByHammingDistance sorts results by Hamming distance (ascending)
func sortResultsByHammingDistance(results []SearchResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].HammingDist > results[j].HammingDist {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// extractComponentName attempts to extract component name from directory path
func extractComponentName(dirPath string) string {
	baseName := filepath.Base(dirPath)

	// Remove common version patterns
	re := regexp.MustCompile(`[-_]v?\d+[\.\d+]*.*$`)
	componentName := re.ReplaceAllString(baseName, "")

	// Clean up the component name
	componentName = strings.ToLower(componentName)
	componentName = strings.Trim(componentName, "-_.")

	return componentName
}

// calculateStringSimilarity calculates simple string similarity (Levenshtein-based)
func calculateStringSimilarity(s1, s2 string) float32 {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if s1 == s2 {
		return 1.0
	}

	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		return 0.8
	}

	// Simple similarity based on common characters
	common := 0
	for _, char := range s1 {
		if strings.ContainsRune(s2, char) {
			common++
		}
	}

	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	if maxLen == 0 {
		return 0.0
	}

	return float32(common) / float32(maxLen)
}
