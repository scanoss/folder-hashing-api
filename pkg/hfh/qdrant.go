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

	// Content-based tie-breaking constants - made much more strict
	TIE_BREAKING_THRESHOLD = 1   // Hamming distance threshold for considering results "tied" (reduced from 3)
	CONTENT_TIE_WEIGHT     = 0.8 // Very heavy weight for content in tie-breaking scenarios (increased from 0.8)
	NAME_DIR_TIE_WEIGHT    = 0.2 // Much lighter weight for name/dir in tie-breaking scenarios (reduced from 0.2)
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

// ComponentGroup represents grouped results by component name with enhanced consensus scoring
type ComponentGroup struct {
	Component      string          `json:"component"`
	Vendor         string          `json:"vendor"`
	BestMatch      VersionResult   `json:"best_match"`
	OtherVersions  []string        `json:"other_versions,omitempty"`
	AllVersions    []VersionResult `json:"all_versions,omitempty"`
	ConsensusScore float32         `json:"consensus_score"` // How many results support this component
	ResultCount    int             `json:"result_count"`    // Total number of results for this component
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

// resolveContentTieBreaking performs content-based tie-breaking for results with similar name/dir scores
func resolveContentTieBreaking(results []SearchResult, contentHash uint64) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	fmt.Printf("  Content tie-breaking: Processing %d results\n", len(results))

	// Group results by similarity level (within TIE_BREAKING_THRESHOLD)
	var tieGroups [][]SearchResult
	processed := make(map[int]bool)

	for i, result := range results {
		if processed[i] {
			continue
		}

		// Create a tie group starting with this result
		tieGroup := []SearchResult{result}
		processed[i] = true

		// Find all results within tie-breaking threshold
		for j := i + 1; j < len(results); j++ {
			if processed[j] {
				continue
			}

			// Check if results are tied (within threshold)
			hammingDiff := results[j].HammingDist - result.HammingDist
			if hammingDiff < 0 {
				hammingDiff = -hammingDiff
			}

			if hammingDiff <= TIE_BREAKING_THRESHOLD {
				tieGroup = append(tieGroup, results[j])
				processed[j] = true
			}
		}

		tieGroups = append(tieGroups, tieGroup)
	}

	fmt.Printf("  Found %d tie groups for content evaluation\n", len(tieGroups))

	// Process each tie group with content-based scoring
	var finalResults []SearchResult

	for groupIdx, tieGroup := range tieGroups {
		if len(tieGroup) == 1 {
			// No tie-breaking needed
			finalResults = append(finalResults, tieGroup[0])
			continue
		}

		fmt.Printf("  Processing tie group %d with %d results\n", groupIdx+1, len(tieGroup))

		// Calculate content similarity for each result in the tie group
		for i := range tieGroup {
			// Get the content hash from metadata or calculate content distance
			var contentSimilarity float32

			// Try to get content hash from metadata
			if contentHashStr, exists := tieGroup[i].Metadata["hfh_contents_hash"]; exists {
				if hashStr, ok := contentHashStr.(string); ok {
					// Parse the stored content hash and calculate similarity
					if len(hashStr) == 16 { // 64-bit hash as 16 hex chars
						// Simple approach: count matching characters (approximation)
						queryHashStr := fmt.Sprintf("%016x", contentHash)
						matches := 0
						for j := 0; j < len(hashStr) && j < len(queryHashStr); j++ {
							if hashStr[j] == queryHashStr[j] {
								matches++
							}
						}
						contentSimilarity = float32(matches) / float32(len(hashStr))
					}
				}
			}

			// Calculate content-weighted score
			nameDirContribution := float32(tieGroup[i].HammingDist) * NAME_DIR_TIE_WEIGHT
			contentContribution := (1.0 - contentSimilarity) * 64.0 * CONTENT_TIE_WEIGHT

			// New tie-breaking score (lower is better)
			tieGroup[i].Score = nameDirContribution + contentContribution

			fmt.Printf("    Result %d: Component=%s, NameDir=%d, ContentSim=%.3f, TieScore=%.3f\n",
				i+1, tieGroup[i].Component, tieGroup[i].HammingDist, contentSimilarity, tieGroup[i].Score)
		}

		// Sort tie group by new content-aware score (lower is better)
		for i := 0; i < len(tieGroup); i++ {
			for j := i + 1; j < len(tieGroup); j++ {
				if tieGroup[i].Score > tieGroup[j].Score {
					tieGroup[i], tieGroup[j] = tieGroup[j], tieGroup[i]
				}
			}
		}

		// Add sorted tie group to final results
		finalResults = append(finalResults, tieGroup...)
	}

	fmt.Printf("  Content tie-breaking completed: %d results processed\n", len(finalResults))
	return finalResults
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

// SearchSimilarProjectsProgressive performs enhanced progressive search with component consensus analysis
func SearchSimilarProjectsProgressive(config QdrantConfig, dirHash, nameHash, contentHash uint64, topK uint64) ([]ComponentGroup, error) {
	// Add comprehensive debug logging
	fmt.Printf("\n=== DEBUG: Starting Progressive Search ===\n")
	fmt.Printf("Query Hashes:\n")
	fmt.Printf("  Directory Hash:  %016x\n", dirHash)
	fmt.Printf("  Names Hash:      %016x\n", nameHash)
	fmt.Printf("  Contents Hash:   %016x\n", contentHash)
	fmt.Printf("  Top K requested: %d\n", topK)
	fmt.Printf("==========================================\n\n")

	// Define much more conservative search stages with much stricter content thresholds
	searchStages := []SearchStage{
		{"Ultra-Conservative", 6, 3, 4, 1, true}, // Very strict content threshold (reduced from 6 to 3)
		{"Conservative", 8, 4, 6, 1, true},       // Still very strict content threshold (reduced from 8 to 5)
		{"Moderate", 10, 6, 8, 1, true},          // Maximum permissible content threshold (reduced from 10 to 8)
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

	// Stage 1: Progressive similarity search - collect results from multiple stages
	fmt.Println("Stage 1: Progressive similarity search...")
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

		fmt.Printf("  %s: Found %d results\n", stage.Name, len(results))

		// Always collect results for later evaluation
		allStageResults = append(allStageResults, results...)

		// Check if this stage meets the quality threshold
		if len(results) >= stage.RequiredResults {
			// Analyze component consensus
			consensusGroups := analyzeComponentConsensus(results)

			// Quality check: ensure we have high-confidence component consensus
			if hasHighConsensusResults(consensusGroups) {
				fmt.Printf("  %s: Meets quality threshold with %d component groups\n", stage.Name, len(consensusGroups))

				// If this is the first valid stage or better than previous, save it
				if len(bestStageResults) == 0 || len(results) > len(bestStageResults) {
					bestStageResults = results
					bestStageName = stage.Name
				}

				// For content-focused stage, return immediately since content is most reliable
				if stage.Name == "Content-Focused" {
					fmt.Printf("  %s: Content match found! Returning immediately\n", stage.Name)
					return consensusGroups, nil
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

		// Apply quality filtering before consensus analysis
		filteredResults := applyQualityGates(bestStageResults)
		fmt.Printf("  After quality filtering: %d results remain\n", len(filteredResults))

		if len(filteredResults) > 0 {
			consensusGroups := analyzeComponentConsensus(filteredResults)
			return consensusGroups, nil
		}
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

		// Apply strict quality filtering for fallback results
		filteredFallback := applyQualityGates(finalResults)
		fmt.Printf("  Fallback after quality filtering: %d results remain\n", len(filteredFallback))

		if len(filteredFallback) > 0 {
			consensusGroups := analyzeComponentConsensus(filteredFallback)
			return consensusGroups, nil
		}
	}

	return nil, fmt.Errorf("no similar projects found with sufficient confidence")
}

// analyzeComponentConsensus analyzes results for component consensus and returns ranked component groups
func analyzeComponentConsensus(results []SearchResult) []ComponentGroup {
	if len(results) == 0 {
		return []ComponentGroup{}
	}

	fmt.Printf("Analyzing component consensus from %d results...\n", len(results))

	// Group results by component
	componentMap := make(map[string][]SearchResult)
	for _, result := range results {
		if result.Component != "" {
			componentMap[result.Component] = append(componentMap[result.Component], result)
		}
	}

	var componentGroups []ComponentGroup

	// Analyze each component group
	for _, componentResults := range componentMap {
		if len(componentResults) == 0 {
			continue
		}

		// Calculate consensus score based on multiple factors
		consensusScore := calculateConsensusScore(componentResults, len(results))

		// Enhance results with similarity scores (using empty queryComponent to avoid bias)
		enhancedResults := enhanceResultsWithSimilarity(componentResults, "", "")

		// Group and rank within component
		groups := groupAndRankResults(enhancedResults, true)

		if len(groups) > 0 {
			group := groups[0] // Take the first (best) group
			group.ConsensusScore = consensusScore
			group.ResultCount = len(componentResults)
			componentGroups = append(componentGroups, group)
		}
	}

	// Sort by consensus score (descending) - components with more supporting evidence ranked higher
	for i := 0; i < len(componentGroups); i++ {
		for j := i + 1; j < len(componentGroups); j++ {
			if componentGroups[i].ConsensusScore < componentGroups[j].ConsensusScore {
				componentGroups[i], componentGroups[j] = componentGroups[j], componentGroups[i]
			}
		}
	}

	fmt.Printf("Component consensus analysis complete. Found %d components:\n", len(componentGroups))
	for i, group := range componentGroups {
		fmt.Printf("  %d. %s: %.3f consensus (%d results)\n", i+1, group.Component, group.ConsensusScore, group.ResultCount)
	}

	return componentGroups
}

// calculateConsensusScore calculates a consensus score for a component based on multiple factors
func calculateConsensusScore(componentResults []SearchResult, totalResults int) float32 {
	if len(componentResults) == 0 || totalResults == 0 {
		return 0.0
	}

	// Factor 1: Frequency (how many results point to this component)
	frequency := float32(len(componentResults)) / float32(totalResults)

	// Factor 2: Quality (average confidence of results for this component)
	var totalConfidence float32
	var minHamming int = 64
	for _, result := range componentResults {
		// Calculate a simple confidence based on Hamming distance
		hammingConfidence := 1.0 - (float32(result.HammingDist) / 64.0)
		totalConfidence += hammingConfidence

		if result.HammingDist < minHamming {
			minHamming = result.HammingDist
		}
	}
	avgQuality := totalConfidence / float32(len(componentResults))

	// Factor 3: Best result quality (bonus for having at least one very good result)
	bestResultBonus := 1.0 - (float32(minHamming) / 64.0)

	// Factor 4: Version diversity (bonus for having multiple versions, indicates established component)
	versionMap := make(map[string]bool)
	for _, result := range componentResults {
		if result.Version != "" {
			versionMap[result.Version] = true
		}
	}
	versionDiversity := float32(len(versionMap)) / float32(len(componentResults))
	if versionDiversity > 1.0 {
		versionDiversity = 1.0
	}

	// Weighted combination
	consensusScore := (frequency * 0.4) + (avgQuality * 0.3) + (bestResultBonus * 0.2) + (versionDiversity * 0.1)

	return consensusScore
}

// hasHighConsensusResults checks if the component groups show strong consensus
func hasHighConsensusResults(groups []ComponentGroup) bool {
	if len(groups) == 0 {
		return false
	}

	// Check if the top component has strong consensus
	topGroup := groups[0]
	if topGroup.ConsensusScore > 0.6 && topGroup.ResultCount >= 2 {
		return true
	}

	// Check if we have multiple components with decent consensus
	decentConsensus := 0
	for _, group := range groups {
		if group.ConsensusScore > 0.4 && group.ResultCount >= 2 {
			decentConsensus++
		}
	}

	return decentConsensus >= 1
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
	fmt.Printf("    === Stage %s Debug ===\n", stage.Name)
	fmt.Printf("    Thresholds: Content=%d, Dirs=%d, Names=%d\n", stage.ContentThreshold, stage.DirsThreshold, stage.NamesThreshold)

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
		fmt.Printf("    Contents search error: %v\n", contentErr)
		contentResults = []SearchResult{}
	}
	if dirErr != nil {
		fmt.Printf("    Dirs search error: %v\n", dirErr)
		dirResults = []SearchResult{}
	}
	if nameErr != nil {
		fmt.Printf("    Names search error: %v\n", nameErr)
		nameResults = []SearchResult{}
	}

	fmt.Printf("    Collection results: Contents=%d, Dirs=%d, Names=%d\n", len(contentResults), len(dirResults), len(nameResults))

	// Debug: log some examples from each collection
	if len(contentResults) > 0 {
		fmt.Printf("    Content results sample: %s (hamming=%d)\n", contentResults[0].Component, contentResults[0].HammingDist)
	}
	if len(dirResults) > 0 {
		fmt.Printf("    Dirs results sample: %s (hamming=%d)\n", dirResults[0].Component, dirResults[0].HammingDist)
	}
	if len(nameResults) > 0 {
		fmt.Printf("    Names results sample: %s (hamming=%d)\n", nameResults[0].Component, nameResults[0].HammingDist)
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

	// Apply content-based tie-breaking for results with similar name/dir scores
	if len(finalResults) > 1 {
		finalResults = resolveContentTieBreaking(finalResults, contentHash)
	}

	// Debug: log final results for this stage
	fmt.Printf("    Final stage results: %d unique components\n", len(finalResults))
	for i, result := range finalResults {
		if i < 5 { // Show first 5 results
			fmt.Printf("      %d. %s %s (hamming=%d, confidence=%.3f)\n",
				i+1, result.Component, result.Version, result.HammingDist, result.ConfidenceScore)
		}
		if i == 5 && len(finalResults) > 5 {
			fmt.Printf("      ... and %d more\n", len(finalResults)-5)
			break
		}
	}

	return finalResults, nil
}

// enhanceResultsWithSimilarity enhances results with confidence scores based purely on hash similarity
func enhanceResultsWithSimilarity(results []SearchResult, queryComponent string, queryVersion string) []SearchResult {
	for i := range results {
		// Calculate component similarity only if provided (for backwards compatibility)
		if queryComponent != "" {
			results[i].ComponentSimilarity = calculateComponentSimilarity(queryComponent, results[i].Component)
		}

		// Calculate version similarity only if provided (for backwards compatibility)
		if queryVersion != "" {
			results[i].VersionSimilarity = calculateVersionSimilarity(queryVersion, results[i].Version)
		}

		// Calculate overall confidence score based primarily on hash similarity
		hammingScore := 1.0 - (float32(results[i].HammingDist) / 64.0) // Normalize Hamming distance

		// If no query component/version provided, use pure hash-based confidence
		if queryComponent == "" && queryVersion == "" {
			results[i].ConfidenceScore = hammingScore
		} else {
			// Legacy mode with component/version similarity (for backwards compatibility)
			results[i].ConfidenceScore = (hammingScore * 0.4) + (results[i].ComponentSimilarity * 0.4) + (results[i].VersionSimilarity * 0.2)
		}
	}

	return results
}

// groupAndRankResults groups results by component and ranks them
func groupAndRankResults(results []SearchResult, useVersionBoost bool) []ComponentGroup {
	groups := make(map[string]*ComponentGroup)

	for _, result := range results {
		key := result.Component

		// Apply version boost if enabled
		confidence := result.ConfidenceScore
		if useVersionBoost {
			// Boost confidence for newer versions or stable versions
			versionBoost := calculateVersionBoost(result.Version)
			confidence = confidence * versionBoost
		}

		versionResult := VersionResult{
			Version:         result.Version,
			HammingDistance: result.HammingDist,
			SearchStage:     result.SearchStage,
			ConfidenceScore: confidence,
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

// calculateVersionBoost calculates a boost factor for versions (newer/stable versions get higher boost)
func calculateVersionBoost(version string) float32 {
	if version == "" {
		return 1.0 // No boost for empty versions
	}

	// Parse the version
	parsedVersion := parseSemanticVersion(version)

	// Base boost is 1.0 (no change)
	boost := float32(1.0)

	// Boost for higher major versions (indicates more mature/recent)
	if parsedVersion.Major >= 2 {
		boost += 0.1 // 10% boost for v2+
	}
	if parsedVersion.Major >= 3 {
		boost += 0.05 // Additional 5% boost for v3+
	}

	// Slight boost for non-zero minor versions (indicates active development)
	if parsedVersion.Minor > 0 {
		boost += 0.02 // 2% boost
	}

	// Penalty for pre-release versions (alpha, beta, rc, etc.)
	if parsedVersion.Pre != "" {
		lowerPre := strings.ToLower(parsedVersion.Pre)
		if strings.Contains(lowerPre, "alpha") || strings.Contains(lowerPre, "a") {
			boost -= 0.15 // 15% penalty for alpha
		} else if strings.Contains(lowerPre, "beta") || strings.Contains(lowerPre, "b") {
			boost -= 0.1 // 10% penalty for beta
		} else if strings.Contains(lowerPre, "rc") || strings.Contains(lowerPre, "release") {
			boost -= 0.05 // 5% penalty for release candidates
		} else {
			boost -= 0.08 // 8% penalty for other pre-release indicators
		}
	}

	// Ensure boost stays within reasonable bounds
	if boost < 0.7 {
		boost = 0.7 // Minimum 70% of original confidence
	}
	if boost > 1.3 {
		boost = 1.3 // Maximum 130% of original confidence
	}

	return boost
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

// applyQualityGates filters results based on quality criteria to reduce false positives
func applyQualityGates(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	fmt.Printf("  Applying quality gates to %d results...\n", len(results))

	var filtered []SearchResult

	// Calculate statistics for quality assessment
	var hammingDistances []int
	for _, result := range results {
		hammingDistances = append(hammingDistances, result.HammingDist)
	}

	// Calculate median Hamming distance for reference
	medianHamming := calculateMedian(hammingDistances)
	fmt.Printf("  Median Hamming distance: %d\n", medianHamming)

	for _, result := range results {
		// Quality Gate 1: Maximum Hamming distance threshold (much more aggressive)
		if result.HammingDist > 10 {
			fmt.Printf("  Filtered out %s (hamming=%d > 10)\n", result.Component, result.HammingDist)
			continue
		}

		// Quality Gate 2: Minimum confidence score based on Hamming distance
		hammingConfidence := 1.0 - (float32(result.HammingDist) / 64.0)
		if hammingConfidence < 0.35 { // Increased from 25% to 35% confidence
			fmt.Printf("  Filtered out %s (confidence=%.3f < 0.35)\n", result.Component, hammingConfidence)
			continue
		}

		// Quality Gate 3: Component name quality check
		if len(result.Component) < 3 { // Increased from 2 to 3 characters
			fmt.Printf("  Filtered out %s (component name too short)\n", result.Component)
			continue
		}

		// Quality Gate 4: Avoid components with suspicious patterns (too generic)
		if isGenericComponentName(result.Component) {
			fmt.Printf("  Filtered out %s (generic component name)\n", result.Component)
			continue
		}

		// Quality Gate 5: Stricter threshold for single-result components
		if result.HammingDist > 8 {
			// For results with hamming > 8, they need to be part of a multi-result component group
			// This will be validated at the component group level
			fmt.Printf("  Flagged %s for group validation (hamming=%d > 8)\n", result.Component, result.HammingDist)
		}

		// Passed all quality gates
		filtered = append(filtered, result)
	}

	fmt.Printf("  Quality gates passed: %d/%d results\n", len(filtered), len(results))
	return filtered
}

// calculateMedian calculates the median of a slice of integers
func calculateMedian(values []int) int {
	if len(values) == 0 {
		return 0
	}

	// Sort the values
	sorted := make([]int, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Return median
	if len(sorted)%2 == 0 {
		return (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	return sorted[len(sorted)/2]
}

// isGenericComponentName checks if a component name is too generic and likely a false positive
func isGenericComponentName(componentName string) bool {
	genericNames := []string{
		"test", "example", "sample", "demo", "temp", "tmp", "data", "lib", "utils", "common",
		"core", "main", "base", "app", "application", "service", "client", "server", "api",
		"tool", "helper", "config", "settings", "default", "generic", "template", "prototype",
	}

	lowername := strings.ToLower(componentName)
	for _, generic := range genericNames {
		if lowername == generic || strings.Contains(lowername, generic) {
			return true
		}
	}

	// Check for very short names (likely too generic)
	if len(componentName) <= 3 {
		return true
	}

	return false
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
