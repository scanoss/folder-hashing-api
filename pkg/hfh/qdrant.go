package hfh

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

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
	Score               float32
	ID                  uint64
	CombinedHash        string
	Vendor              string
	Component           string
	Version             string
	URL                 string
	Metadata            map[string]interface{}
	SearchStage         string  // Which stage found this result
	HammingDist         int     // Hamming distance for this result
	ComponentSimilarity float32 // Component name similarity score
	VersionSimilarity   float32 // Version similarity score
	ConfidenceScore     float32 // Overall confidence score
}

// ComponentGroup represents grouped results by component name
type ComponentGroup struct {
	Component     string          `json:"component"`
	Vendor        string          `json:"vendor"`
	BestMatch     VersionResult   `json:"best_match"`
	OtherVersions []string        `json:"other_versions,omitempty"`
	AllVersions   []VersionResult `json:"all_versions,omitempty"`
}

// VersionResult represents a version-specific result within a component group
type VersionResult struct {
	Version         string                 `json:"version"`
	HammingDistance int                    `json:"hamming_distance"`
	SearchStage     string                 `json:"search_stage"`
	ConfidenceScore float32                `json:"confidence_score"`
	URL             string                 `json:"url,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// SearchStage defines a progressive search stage with specific thresholds
type SearchStage struct {
	Name             string
	ContentThreshold int
	DirsThreshold    int
	NamesThreshold   int
	MinComponentSim  float32
	RequiredResults  int
	UseVersionBoost  bool
}

// SemanticVersion represents a parsed semantic version
type SemanticVersion struct {
	Major int
	Minor int
	Patch int
	Pre   string
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
	log.Println("Checking collections existence...")
	log.Println("Dirs collection name:", config.DirsCollectionName)
	log.Println("Names collection name:", config.NamesCollectionName)
	log.Println("Contents collection name:", config.ContentsCollectionName)
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

// SearchSimilarProjectsProgressive performs enhanced progressive search with component filtering
func SearchSimilarProjectsProgressive(config QdrantConfig, dirHash, nameHash, contentHash uint64, queryComponent string, topK uint64) ([]ComponentGroup, error) {
	// Define progressive search stages
	searchStages := []SearchStage{
		{"Content-Focused", 15, 6, 8, 0.8, 1, true},    // Prioritize content matches first
		{"Ultra-Conservative", 10, 6, 8, 0.8, 2, true}, // Relaxed content threshold
		{"Conservative", 13, 8, 10, 0.6, 2, true},      // Content ≤13 for v1.17.3
		{"Moderate", 15, 12, 15, 0.4, 1, true},         // Even more relaxed
		{"Relaxed", 18, 15, 20, 0.2, 1, false},         // Very relaxed
		{"Fallback", 25, 18, 25, 0.0, 1, false},        // Catch-all
	}

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

	// Stage 1: Try exact match first
	fmt.Println("Stage 1: Searching for exact hash matches...")
	dirHashStr := fmt.Sprintf("%016x", dirHash)
	nameHashStr := fmt.Sprintf("%016x", nameHash)
	contentHashStr := fmt.Sprintf("%016x", contentHash)

	exactResults, err := searchExactMatchesMultiIndex(ctx, client, config, dirHashStr, nameHashStr, contentHashStr, topK)
	if err != nil {
		fmt.Printf("Warning: Stage 1 search failed: %v\n", err)
	} else if len(exactResults) > 0 {
		fmt.Printf("Stage 1: Found %d exact matches\n", len(exactResults))
		// Enhance results with component similarity
		enhancedResults := enhanceResultsWithSimilarity(exactResults, queryComponent, "")
		return groupAndRankResults(enhancedResults, true), nil
	}

	// Stage 2: Progressive similarity search - collect results from multiple stages
	fmt.Println("Stage 2: Progressive similarity search...")
	var allStageResults []SearchResult
	bestStageResults := []SearchResult{}
	bestStageName := ""

	for i, stage := range searchStages {
		fmt.Printf("  Trying %s (Stage %d/%d)...\n", stage.Name, i+1, len(searchStages))

		results, err := searchWithStageThresholds(ctx, client, config, dirHash, nameHash, contentHash, stage, topK*2)
		if err != nil {
			fmt.Printf("  Warning: %s search failed: %v\n", stage.Name, err)
			continue
		}

		// Filter by component similarity
		filteredResults := filterByComponentSimilarity(results, queryComponent, stage.MinComponentSim)

		fmt.Printf("  %s: Found %d results (after component filtering)\n", stage.Name, len(filteredResults))

		// Always collect results for later evaluation
		allStageResults = append(allStageResults, filteredResults...)

		// Check if this stage meets the quality threshold
		if len(filteredResults) >= stage.RequiredResults {
			// Enhance with similarity scores
			enhancedResults := enhanceResultsWithSimilarity(filteredResults, queryComponent, "")
			grouped := groupAndRankResults(enhancedResults, stage.UseVersionBoost)

			// Quality check: ensure we have high-confidence results
			if hasHighConfidenceResults(grouped) {
				fmt.Printf("  %s: Meets quality threshold with %d component groups\n", stage.Name, len(grouped))

				// If this is the first valid stage or better than previous, save it
				if len(bestStageResults) == 0 || len(filteredResults) > len(bestStageResults) {
					bestStageResults = filteredResults
					bestStageName = stage.Name
				}

				// For content-focused stage, return immediately since content is most reliable
				if stage.Name == "Content-Focused" {
					fmt.Printf("  %s: Content match found! Returning immediately\n", stage.Name)
					return grouped, nil
				}
			}
		}

		// Stop after moderate stage if we have good results to avoid too much noise
		if stage.Name == "Moderate" && len(bestStageResults) > 0 {
			fmt.Printf("  Stopping at Moderate stage - found %d results in %s\n", len(bestStageResults), bestStageName)
			break
		}
	}

	// If we have results from any stage, use the best ones
	if len(bestStageResults) > 0 {
		fmt.Printf("  Using best results from %s stage (%d results)\n", bestStageName, len(bestStageResults))
		enhancedResults := enhanceResultsWithSimilarity(bestStageResults, queryComponent, "")
		grouped := groupAndRankResults(enhancedResults, true)
		return grouped, nil
	}

	// Fallback: if no good results from early stages, try all collected results
	if len(allStageResults) > 0 {
		fmt.Printf("  Fallback: Using all collected results (%d total)\n", len(allStageResults))
		// Remove duplicates from all results
		uniqueResults := make(map[uint64]SearchResult)
		for _, result := range allStageResults {
			if existing, exists := uniqueResults[result.ID]; exists {
				// Keep the one with lower Hamming distance
				if result.HammingDist < existing.HammingDist {
					uniqueResults[result.ID] = result
				}
			} else {
				uniqueResults[result.ID] = result
			}
		}

		// Convert back to slice
		var finalResults []SearchResult
		for _, result := range uniqueResults {
			finalResults = append(finalResults, result)
		}

		enhancedResults := enhanceResultsWithSimilarity(finalResults, queryComponent, "")
		grouped := groupAndRankResults(enhancedResults, true)
		return grouped, nil
	}

	return nil, fmt.Errorf("no similar projects found with sufficient confidence")
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

// searchMultiIndexSimilarity performs similarity search across all collections with weighted fusion and parallel execution
func searchMultiIndexSimilarity(ctx context.Context, client *qdrant.Client, config QdrantConfig, dirHash, nameHash, contentHash uint64, limit uint64) ([]SearchResult, error) {
	// Create individual query vectors for each hash type
	dirVector := hashToVector(dirHash)
	nameVector := hashToVector(nameHash)
	contentVector := hashToVector(contentHash)

	// Search each collection with appropriate thresholds (aligned with pseudocode)
	searchLimit := limit * 2 // Get more results to allow for fusion and filtering

	// Search all collections in parallel
	var wg sync.WaitGroup
	var contentResults, dirResults, nameResults []SearchResult
	var contentErr, dirErr, nameErr error

	fmt.Println("Searching all collections in parallel...")

	wg.Add(3)

	// Search contents collection (code content similarity) - threshold 10
	go func() {
		defer wg.Done()
		fmt.Printf("Searching contents collection (threshold=10)...")
		contentResults, contentErr = searchSingleCollection(ctx, client, config.ContentsCollectionName, contentVector, searchLimit, 10, "Contents")
	}()

	// Search dirs collection (structural similarity) - threshold 15
	go func() {
		defer wg.Done()
		fmt.Printf("Searching dirs collection (threshold=15)...")
		dirResults, dirErr = searchSingleCollection(ctx, client, config.DirsCollectionName, dirVector, searchLimit, 15, "Dirs")
	}()

	// Search names collection (component naming similarity) - threshold 20
	go func() {
		defer wg.Done()
		fmt.Printf("Searching names collection (threshold=20)...")
		nameResults, nameErr = searchSingleCollection(ctx, client, config.NamesCollectionName, nameVector, searchLimit, 20, "Names")
	}()

	wg.Wait()

	// Handle errors from parallel searches
	if contentErr != nil {
		fmt.Printf("Warning: Contents collection search failed: %v\n", contentErr)
		contentResults = []SearchResult{}
	}
	if dirErr != nil {
		fmt.Printf("Warning: Dirs collection search failed: %v\n", dirErr)
		dirResults = []SearchResult{}
	}
	if nameErr != nil {
		fmt.Printf("Warning: Names collection search failed: %v\n", nameErr)
		nameResults = []SearchResult{}
	}

	fmt.Printf("Parallel search completed. Results: contents=%d, dirs=%d, names=%d\n",
		len(contentResults), len(dirResults), len(nameResults))

	// Perform weighted fusion of results (content=0.5, dir=0.3, name=0.2)
	fusedResults := fuseSearchResults(dirResults, nameResults, contentResults, 0.3, 0.2, 0.5)

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

// ExtractComponentNameFromPath extracts component name from directory path (public function)
func ExtractComponentNameFromPath(dirPath string) string {
	return extractComponentName(dirPath)
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

// calculateComponentSimilarity calculates advanced component name similarity
func calculateComponentSimilarity(comp1, comp2 string) float32 {
	// Exact match
	if comp1 == comp2 {
		return 1.0
	}

	// Normalize components (lowercase, remove common suffixes)
	norm1 := normalizeComponentName(comp1)
	norm2 := normalizeComponentName(comp2)

	if norm1 == norm2 {
		return 0.95
	}

	// Calculate Levenshtein similarity
	levSim := levenshteinSimilarity(norm1, norm2)

	// Bonus for shared package paths (e.g., org.apache.*)
	packageSim := calculatePackageSimilarity(comp1, comp2)

	// Weighted combination
	return (levSim * 0.7) + (packageSim * 0.3)
}

// normalizeComponentName normalizes component names for better comparison
func normalizeComponentName(component string) string {
	normalized := strings.ToLower(component)

	// Remove common prefixes/suffixes
	suffixes := []string{"-core", "-main", "-api", "-lib", "-client", "-server", ".jar", ".war"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(normalized, suffix) {
			normalized = strings.TrimSuffix(normalized, suffix)
			break
		}
	}

	return normalized
}

// levenshteinSimilarity calculates Levenshtein similarity (0-1)
func levenshteinSimilarity(s1, s2 string) float32 {
	len1, len2 := len(s1), len(s2)
	if len1 == 0 {
		return float32(len2)
	}
	if len2 == 0 {
		return float32(len1)
	}

	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	maxLen := len1
	if len2 > maxLen {
		maxLen = len2
	}

	distance := matrix[len1][len2]
	return 1.0 - float32(distance)/float32(maxLen)
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// calculatePackageSimilarity calculates similarity based on package structure
func calculatePackageSimilarity(comp1, comp2 string) float32 {
	parts1 := strings.Split(comp1, ".")
	parts2 := strings.Split(comp2, ".")

	commonParts := 0
	maxLen := len(parts1)
	if len(parts2) < maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		if parts1[i] == parts2[i] {
			commonParts++
		} else {
			break // Stop at first difference for package hierarchy
		}
	}

	totalParts := len(parts1)
	if len(parts2) > totalParts {
		totalParts = len(parts2)
	}

	if totalParts == 0 {
		return 0.0
	}

	return float32(commonParts) / float32(totalParts)
}

// searchWithStageThresholds performs search with specific stage thresholds
func searchWithStageThresholds(ctx context.Context, client *qdrant.Client, config QdrantConfig, dirHash, nameHash, contentHash uint64, stage SearchStage, limit uint64) ([]SearchResult, error) {
	// Create individual query vectors for each hash type
	dirVector := hashToVector(dirHash)
	nameVector := hashToVector(nameHash)
	contentVector := hashToVector(contentHash)

	// Search all collections in parallel with stage-specific thresholds
	var wg sync.WaitGroup
	var contentResults, dirResults, nameResults []SearchResult
	var contentErr, dirErr, nameErr error

	wg.Add(3)

	// Search contents collection with stage threshold
	go func() {
		defer wg.Done()
		contentResults, contentErr = searchSingleCollection(ctx, client, config.ContentsCollectionName, contentVector, limit, stage.ContentThreshold, "Contents")
	}()

	// Search dirs collection with stage threshold
	go func() {
		defer wg.Done()
		dirResults, dirErr = searchSingleCollection(ctx, client, config.DirsCollectionName, dirVector, limit, stage.DirsThreshold, "Dirs")
	}()

	// Search names collection with stage threshold
	go func() {
		defer wg.Done()
		nameResults, nameErr = searchSingleCollection(ctx, client, config.NamesCollectionName, nameVector, limit, stage.NamesThreshold, "Names")
	}()

	wg.Wait()

	// Handle errors from parallel searches
	if contentErr != nil {
		contentResults = []SearchResult{}
	}
	if dirErr != nil {
		dirResults = []SearchResult{}
	}
	if nameErr != nil {
		nameResults = []SearchResult{}
	}

	// Combine and fuse results
	allResults := append(contentResults, append(dirResults, nameResults...)...)

	// Remove duplicates and enhance with stage info
	uniqueResults := make(map[uint64]SearchResult)
	for _, result := range allResults {
		if existing, exists := uniqueResults[result.ID]; exists {
			// Keep the one with lower Hamming distance
			if result.HammingDist < existing.HammingDist {
				result.SearchStage = fmt.Sprintf("%s: %s", stage.Name, result.SearchStage)
				uniqueResults[result.ID] = result
			}
		} else {
			result.SearchStage = fmt.Sprintf("%s: %s", stage.Name, result.SearchStage)
			uniqueResults[result.ID] = result
		}
	}

	// Convert back to slice
	var finalResults []SearchResult
	for _, result := range uniqueResults {
		finalResults = append(finalResults, result)
	}

	// Sort by Hamming distance
	sortResultsByHammingDistance(finalResults)

	return finalResults, nil
}

// filterByComponentSimilarity filters results based on component name similarity
func filterByComponentSimilarity(results []SearchResult, queryComponent string, minSimilarity float32) []SearchResult {
	if queryComponent == "" || minSimilarity <= 0 {
		return results // No filtering if no query component or threshold
	}

	var filteredResults []SearchResult
	for _, result := range results {
		compSim := calculateComponentSimilarity(queryComponent, result.Component)
		if compSim >= minSimilarity {
			result.ComponentSimilarity = compSim
			filteredResults = append(filteredResults, result)
		}
	}

	return filteredResults
}

// enhanceResultsWithSimilarity enhances results with component and version similarity scores
func enhanceResultsWithSimilarity(results []SearchResult, queryComponent string, queryVersion string) []SearchResult {
	for i := range results {
		// Calculate component similarity
		if queryComponent != "" {
			results[i].ComponentSimilarity = calculateComponentSimilarity(queryComponent, results[i].Component)
		}

		// Calculate version similarity
		if queryVersion != "" {
			results[i].VersionSimilarity = calculateVersionSimilarity(queryVersion, results[i].Version)
		}

		// Calculate overall confidence score
		hammingScore := 1.0 - (float32(results[i].HammingDist) / 64.0) // Normalize Hamming distance
		results[i].ConfidenceScore = (hammingScore * 0.4) + (results[i].ComponentSimilarity * 0.4) + (results[i].VersionSimilarity * 0.2)
	}

	return results
}

// groupAndRankResults groups results by component and ranks them
func groupAndRankResults(results []SearchResult, useVersionBoost bool) []ComponentGroup {
	groups := make(map[string]*ComponentGroup)

	for _, result := range results {
		key := result.Component

		versionResult := VersionResult{
			Version:         result.Version,
			HammingDistance: result.HammingDist,
			SearchStage:     result.SearchStage,
			ConfidenceScore: result.ConfidenceScore,
			URL:             result.URL,
			Metadata:        result.Metadata,
		}

		if group, exists := groups[key]; exists {
			// Add version to existing component group
			group.AllVersions = append(group.AllVersions, versionResult)

			// Update best match if this version is better
			if versionResult.ConfidenceScore > group.BestMatch.ConfidenceScore {
				group.BestMatch = versionResult
			}
		} else {
			// Create new component group
			groups[key] = &ComponentGroup{
				Component:   result.Component,
				Vendor:      result.Vendor,
				BestMatch:   versionResult,
				AllVersions: []VersionResult{versionResult},
			}
		}
	}

	// Convert to slice and sort by best match confidence
	var groupSlice []ComponentGroup
	for _, group := range groups {
		// Sort versions within group by confidence score
		for i := 0; i < len(group.AllVersions); i++ {
			for j := i + 1; j < len(group.AllVersions); j++ {
				if group.AllVersions[i].ConfidenceScore < group.AllVersions[j].ConfidenceScore {
					group.AllVersions[i], group.AllVersions[j] = group.AllVersions[j], group.AllVersions[i]
				}
			}
		}

		// Create other versions list (excluding best match)
		for _, version := range group.AllVersions {
			if version.Version != group.BestMatch.Version {
				group.OtherVersions = append(group.OtherVersions, version.Version)
			}
		}

		groupSlice = append(groupSlice, *group)
	}

	// Sort groups by best match confidence score (descending)
	for i := 0; i < len(groupSlice); i++ {
		for j := i + 1; j < len(groupSlice); j++ {
			if groupSlice[i].BestMatch.ConfidenceScore < groupSlice[j].BestMatch.ConfidenceScore {
				groupSlice[i], groupSlice[j] = groupSlice[j], groupSlice[i]
			}
		}
	}

	return groupSlice
}

// hasHighConfidenceResults checks if the results contain high-confidence matches
func hasHighConfidenceResults(groups []ComponentGroup) bool {
	if len(groups) == 0 {
		return false
	}

	// Check if the best result has high confidence
	bestGroup := groups[0]
	if bestGroup.BestMatch.ConfidenceScore > 0.6 {
		return true
	}

	// Check if we have multiple decent results
	decentResults := 0
	for _, group := range groups {
		if group.BestMatch.ConfidenceScore > 0.4 {
			decentResults++
		}
	}

	return decentResults >= 2
}

// calculateVersionSimilarity calculates semantic version similarity
func calculateVersionSimilarity(v1, v2 string) float32 {
	if v1 == v2 {
		return 1.0
	}

	ver1 := parseSemanticVersion(v1)
	ver2 := parseSemanticVersion(v2)

	// Same major.minor = high similarity
	if ver1.Major == ver2.Major && ver1.Minor == ver2.Minor {
		patchDiff := ver1.Patch - ver2.Patch
		if patchDiff < 0 {
			patchDiff = -patchDiff
		}
		return 0.9 - float32(patchDiff)*0.05 // Reduce by 0.05 per patch difference
	}

	// Same major = medium similarity
	if ver1.Major == ver2.Major {
		minorDiff := ver1.Minor - ver2.Minor
		if minorDiff < 0 {
			minorDiff = -minorDiff
		}
		return 0.7 - float32(minorDiff)*0.1 // Reduce by 0.1 per minor difference
	}

	// Different major = low similarity
	majorDiff := ver1.Major - ver2.Major
	if majorDiff < 0 {
		majorDiff = -majorDiff
	}
	similarity := 0.3 - float32(majorDiff)*0.1
	if similarity < 0 {
		similarity = 0
	}
	return similarity
}

// parseSemanticVersion parses a semantic version string
func parseSemanticVersion(version string) SemanticVersion {
	// Clean version string
	version = strings.TrimSpace(version)

	// Remove 'v' prefix if present
	if strings.HasPrefix(version, "v") {
		version = version[1:]
	}

	// Split by dots
	parts := strings.Split(version, ".")

	result := SemanticVersion{Major: 0, Minor: 0, Patch: 0}

	// Parse major
	if len(parts) > 0 {
		if major, err := parseInt(parts[0]); err == nil {
			result.Major = major
		}
	}

	// Parse minor
	if len(parts) > 1 {
		if minor, err := parseInt(parts[1]); err == nil {
			result.Minor = minor
		}
	}

	// Parse patch (handle pre-release suffixes)
	if len(parts) > 2 {
		patchStr := parts[2]
		// Split on common pre-release separators
		for _, sep := range []string{"-", "_", "+", "."} {
			if idx := strings.Index(patchStr, sep); idx >= 0 {
				result.Pre = patchStr[idx:]
				patchStr = patchStr[:idx]
				break
			}
		}

		if patch, err := parseInt(patchStr); err == nil {
			result.Patch = patch
		}
	}

	return result
}

// parseInt safely parses integer from string
func parseInt(s string) (int, error) {
	// Remove any non-digit characters at the end
	cleaned := ""
	for _, r := range s {
		if r >= '0' && r <= '9' {
			cleaned += string(r)
		} else {
			break
		}
	}

	if cleaned == "" {
		return 0, fmt.Errorf("no digits found")
	}

	result := 0
	for _, r := range cleaned {
		result = result*10 + int(r-'0')
	}

	return result, nil
}
