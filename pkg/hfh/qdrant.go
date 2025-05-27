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
	VectorDim = 64 // Single 64-bit hash per collection
)

// QdrantConfig holds Qdrant connection configuration for multi-index approach
type QdrantConfig struct {
	Host                   string
	Port                   int
	CollectionBaseName     string // Base name for collection family
	DirsCollectionName     string
	NamesCollectionName    string
	ContentsCollectionName string
}

// NewQdrantConfig creates a new QdrantConfig with collection names set
func NewQdrantConfig(host string, port int, baseName string) QdrantConfig {
	return QdrantConfig{
		Host:                   host,
		Port:                   port,
		CollectionBaseName:     baseName,
		DirsCollectionName:     baseName + "_dirs",
		NamesCollectionName:    baseName + "_names",
		ContentsCollectionName: baseName + "_contents",
	}
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

// SearchSimilarProjects searches for similar projects using multi-index approach with weighted fusion
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

	// Check if all collections exist
	ctx := context.Background()
	dirsExists, err := client.CollectionExists(ctx, config.DirsCollectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking dirs collection existence: %v", err)
	}
	namesExists, err := client.CollectionExists(ctx, config.NamesCollectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking names collection existence: %v", err)
	}
	contentsExists, err := client.CollectionExists(ctx, config.ContentsCollectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking contents collection existence: %v", err)
	}

	if !dirsExists || !namesExists || !contentsExists {
		return nil, fmt.Errorf("one or more collections do not exist: dirs=%t, names=%t, contents=%t",
			dirsExists, namesExists, contentsExists)
	}

	// Convert hashes to hex strings for exact matching
	dirHashStr := fmt.Sprintf("%016x", dirHash)
	nameHashStr := fmt.Sprintf("%016x", nameHash)
	contentHashStr := fmt.Sprintf("%016x", contentHash)

	var allResults []SearchResult

	// Stage 1: Exact hash matching across all collections
	fmt.Println("Stage 1: Searching for exact hash matches...")
	exactResults, err := searchExactMatchesMultiIndex(ctx, client, config, dirHashStr, nameHashStr, contentHashStr, topK)
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

	// Stage 2: Multi-index similarity search with weighted fusion
	fmt.Println("Stage 2: Multi-index similarity search...")
	similarityResults, err := searchMultiIndexSimilarity(ctx, client, config, dirHash, nameHash, contentHash, topK-uint64(len(allResults)))
	if err != nil {
		fmt.Printf("Warning: Stage 2 search failed: %v\n", err)
	} else if len(similarityResults) > 0 {
		fmt.Printf("Stage 2: Found %d similarity matches\n", len(similarityResults))
		allResults = append(allResults, similarityResults...)
	} else {
		fmt.Println("Stage 2: No similarity matches found")
	}

	// Ensure we don't exceed topK
	if len(allResults) > int(topK) {
		allResults = allResults[:topK]
	}

	return allResults, nil
}

// searchExactMatchesMultiIndex performs exact hash matching across all collections
func searchExactMatchesMultiIndex(ctx context.Context, client *qdrant.Client, config QdrantConfig, dirHash, nameHash, contentHash string, limit uint64) ([]SearchResult, error) {
	// Search for exact matches by filtering on hash fields in any one collection (they all have the same metadata)
	// We'll use the contents collection as it's likely to have the most distinctive matches
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

	// Scroll through results with filter (using contents collection)
	scrollResp, err := client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: config.ContentsCollectionName,
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

// searchMultiIndexSimilarity performs similarity search across all collections with weighted fusion
func searchMultiIndexSimilarity(ctx context.Context, client *qdrant.Client, config QdrantConfig, dirHash, nameHash, contentHash uint64, limit uint64) ([]SearchResult, error) {
	// Create individual query vectors for each hash type
	dirVector := hashToVector(dirHash)
	nameVector := hashToVector(nameHash)
	contentVector := hashToVector(contentHash)

	// Search each collection with appropriate thresholds
	searchLimit := limit * 2 // Get more results to allow for fusion and filtering

	// Search dirs collection (structural similarity)
	fmt.Printf("Searching dirs collection with threshold...")
	dirResults, err := searchSingleCollection(ctx, client, config.DirsCollectionName, dirVector, searchLimit, 20, "Dirs")
	if err != nil {
		fmt.Printf("Warning: Dirs collection search failed: %v\n", err)
		dirResults = []SearchResult{}
	}

	// Search names collection (component naming similarity)
	fmt.Printf("Searching names collection...")
	nameResults, err := searchSingleCollection(ctx, client, config.NamesCollectionName, nameVector, searchLimit, 25, "Names")
	if err != nil {
		fmt.Printf("Warning: Names collection search failed: %v\n", err)
		nameResults = []SearchResult{}
	}

	// Search contents collection (code content similarity)
	fmt.Printf("Searching contents collection...")
	contentResults, err := searchSingleCollection(ctx, client, config.ContentsCollectionName, contentVector, searchLimit, 15, "Contents")
	if err != nil {
		fmt.Printf("Warning: Contents collection search failed: %v\n", err)
		contentResults = []SearchResult{}
	}

	// Perform weighted fusion of results
	fusedResults := fuseSearchResults(dirResults, nameResults, contentResults, 0.3, 0.2, 0.5) // Contents weighted highest

	// Sort by combined score and return top results
	if len(fusedResults) > int(limit) {
		fusedResults = fusedResults[:limit]
	}

	return fusedResults, nil
}

// searchSingleCollection performs similarity search on a single collection
func searchSingleCollection(ctx context.Context, client *qdrant.Client, collectionName string, queryVector []float32, limit uint64, hammingThreshold int, collectionType string) ([]SearchResult, error) {
	queryReq := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(true), // Need vectors to calculate Hamming distance
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error in %s collection search: %v", collectionType, err)
	}

	var results []SearchResult
	for _, point := range searchResp {
		// Calculate Hamming distance
		pointVector := point.Vectors.GetVector().GetData()
		hammingDist := calculateHammingDistance(queryVector, pointVector)

		// Apply Hamming distance threshold
		if hammingDist <= hammingThreshold {
			result := convertScoredPointToResult(point, fmt.Sprintf("Stage 2: %s Similarity", collectionType), hammingDist)
			results = append(results, result)
		}
	}

	// Sort by Hamming distance (lower is better)
	sortResultsByHammingDistance(results)

	return results, nil
}

// fuseSearchResults combines results from different collections with weighted scoring
func fuseSearchResults(dirResults, nameResults, contentResults []SearchResult, dirWeight, nameWeight, contentWeight float32) []SearchResult {
	// Create a map to combine results by ID
	resultMap := make(map[uint64]*SearchResult)

	// Process dirs results
	for _, result := range dirResults {
		if existing, exists := resultMap[result.ID]; exists {
			// Combine scores with weighting
			existing.Score += (1.0 / float32(result.HammingDist+1)) * dirWeight
			existing.SearchStage += " + Dirs"
		} else {
			// Create new result with weighted score
			newResult := result
			newResult.Score = (1.0 / float32(result.HammingDist+1)) * dirWeight
			newResult.SearchStage = "Multi-Index: Dirs"
			resultMap[result.ID] = &newResult
		}
	}

	// Process names results
	for _, result := range nameResults {
		if existing, exists := resultMap[result.ID]; exists {
			existing.Score += (1.0 / float32(result.HammingDist+1)) * nameWeight
			existing.SearchStage += " + Names"
		} else {
			newResult := result
			newResult.Score = (1.0 / float32(result.HammingDist+1)) * nameWeight
			newResult.SearchStage = "Multi-Index: Names"
			resultMap[result.ID] = &newResult
		}
	}

	// Process contents results
	for _, result := range contentResults {
		if existing, exists := resultMap[result.ID]; exists {
			existing.Score += (1.0 / float32(result.HammingDist+1)) * contentWeight
			existing.SearchStage += " + Contents"
		} else {
			newResult := result
			newResult.Score = (1.0 / float32(result.HammingDist+1)) * contentWeight
			newResult.SearchStage = "Multi-Index: Contents"
			resultMap[result.ID] = &newResult
		}
	}

	// Convert map to slice and sort by score (higher is better)
	var fusedResults []SearchResult
	for _, result := range resultMap {
		fusedResults = append(fusedResults, *result)
	}

	// Sort by score (descending)
	for i := 0; i < len(fusedResults); i++ {
		for j := i + 1; j < len(fusedResults); j++ {
			if fusedResults[i].Score < fusedResults[j].Score {
				fusedResults[i], fusedResults[j] = fusedResults[j], fusedResults[i]
			}
		}
	}

	return fusedResults
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
