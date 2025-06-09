package hfh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 64 // Single 64-bit hash per collection
)

// Language-based collection names (new approach)
var PrimaryLanguages = map[string]string{
	"js":     "javascript_collection",
	"jsx":    "javascript_collection",
	"ts":     "javascript_collection",
	"tsx":    "javascript_collection",
	"py":     "python_collection",
	"java":   "java_collection",
	"class":  "java_collection",
	"jar":    "java_collection",
	"c":      "c_collection",
	"h":      "c_collection",
	"cpp":    "cpp_collection",
	"cxx":    "cpp_collection",
	"cc":     "cpp_collection",
	"hpp":    "cpp_collection",
	"hxx":    "cpp_collection",
	"go":     "go_collection",
	"rb":     "ruby_collection",
	"php":    "php_collection",
	"cs":     "csharp_collection",
	"rs":     "rust_collection",
	"scala":  "scala_collection",
	"kt":     "kotlin_collection",
	"swift":  "swift_collection",
	"sh":     "shell_collection",
	"bash":   "shell_collection",
	"zsh":    "shell_collection",
	"html":   "web_collection",
	"css":    "web_collection",
	"scss":   "web_collection",
	"less":   "web_collection",
	"vue":    "web_collection",
	"svelte": "web_collection",
	"dart":   "dart_collection",
	"sql":    "sql_collection",
	"lua":    "lua_collection",
	"r":      "r_collection",
	"":       "misc_collection", // Files without extension
}

var IndexedLangExtensions = []string{
	// Web/Frontend
	"ts", "js", "jsx", "tsx", "html", "css", "scss", "less", "vue", "svelte",
	// Backend/General
	"py", "java", "class", "jar", "go", "rb", "php", "cs", "rs", "scala", "kt", "groovy", "clj", "ex", "exs",
	// C-family
	"c", "h", "cpp", "cxx", "cc", "hpp", "hxx", "m", "mm", "swift",
	// Shell/Scripts
	"sh", "bash", "zsh", "ps1", "bat", "cmd", "pl", "pm", "t",
	// Data/Config
	"json", "yaml", "yml", "xml", "toml", "ini", "conf", "cfg", "properties",
	// Documentation
	"md", "rst", "txt", "tex", "adoc", "wiki",
	// Mobile
	"dart", "kotlin", "swift", "gradle",
	// Database
	"sql", "graphql", "prisma",
	// Other
	"lua", "r", "d", "fs", "f", "f90", "hs", "erl", "elm", "lisp", "jl",
	// Empty extension (for files without extension)
	"",
}

// QdrantSeparateConfig holds configuration for separate collections approach
type QdrantSeparateConfig struct {
	Host string
	Port int
}

type LanguageExtensions map[string]int32

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Score              float32            `json:"score"`
	ID                 uint64             `json:"id"`
	Vendor             string             `json:"vendor"`
	Component          string             `json:"component"`
	Purl               string             `json:"purl"`
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
	Score              float32            `json:"score"`
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

// GetPrimaryLanguageFromExtensions determines the most common language from extension counts
func GetPrimaryLanguageFromExtensions(langExt LanguageExtensions) string {
	if len(langExt) == 0 {
		return "misc"
	}

	maxCount := int32(0)
	primaryLang := "misc"

	// Find the extension with the highest count
	for ext, count := range langExt {
		if count > maxCount {
			if collectionName, exists := PrimaryLanguages[ext]; exists {
				maxCount = count
				primaryLang = strings.TrimSuffix(collectionName, "_collection")
			}
		}
	}

	return primaryLang
}

// GetCollectionNameFromLanguageExtensions gets the target collection based on language extensions
func GetCollectionNameFromLanguageExtensions(langExt LanguageExtensions) string {
	primaryLang := GetPrimaryLanguageFromExtensions(langExt)
	return primaryLang + "_collection"
}

// GetAllSupportedCollections returns all unique collection names from the PrimaryLanguages map
func GetAllSupportedCollections() []string {
	collectionsMap := make(map[string]bool)
	for _, collectionName := range PrimaryLanguages {
		collectionsMap[collectionName] = true
	}

	collections := make([]string, 0, len(collectionsMap))
	for collectionName := range collectionsMap {
		collections = append(collections, collectionName)
	}

	return collections
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
	for extension, count := range queryLangExt {
		if extension == "" {
			continue
		}
		// If extension is not in IndexedLangExtensions, skip it
		if !slices.Contains(IndexedLangExtensions, extension) {
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
func convertPointToResult(point *qdrant.ScoredPoint) SearchResult {
	result := SearchResult{
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

// SearchLanguageBasedApproximate performs optimized search using RRF
func SearchLanguageBasedApproximate(config QdrantSeparateConfig, dirHash, nameHash, contentHash string, queryLangExt LanguageExtensions, topK uint64) ([]ComponentGroup, error) {
	log.Printf("Starting language-based search with RRF fusion")

	// Determine target collection based on primary language
	collectionName := GetCollectionNameFromLanguageExtensions(queryLangExt)
	log.Printf("Using collection: %s for language extensions: %v", collectionName, queryLangExt)

	// Create a single shared client
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %v", err)
	}
	defer client.Close()

	// Set context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Check if collection exists
	exists, err := client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking collection %s: %v", collectionName, err)
	}
	if !exists {
		log.Printf("Collection %s does not exist, falling back to misc_collection", collectionName)
		collectionName = "misc_collection"

		// Check if misc collection exists
		exists, err = client.CollectionExists(ctx, collectionName)
		if err != nil || !exists {
			return nil, fmt.Errorf("fallback collection %s also does not exist", collectionName)
		}
	}

	// Convert hashes to vectors
	dirVector, err := HexSimhashToVector(dirHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting dir hash to vector: %v", err)
	}
	nameVector, err := HexSimhashToVector(nameHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting name hash to vector: %v", err)
	}
	contentVector, err := HexSimhashToVector(contentHash, VectorDim)
	if err != nil {
		return nil, fmt.Errorf("error converting content hash to vector: %v", err)
	}

	var filters *qdrant.Filter
	mustConditions := []*qdrant.Condition{}
	mustNotConditions := []*qdrant.Condition{}
	shouldConditions := []*qdrant.Condition{}

	if len(queryLangExt) > 0 {
		langExtConditions := buildLanguageExtensionFilter(queryLangExt, 30.0)
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
			Query:  qdrant.NewQuery(nameVector...),
			Using:  qdrant.PtrOf("names"),
			Filter: filters,
		},
		{
			// Dirs vector query
			Query:  qdrant.NewQuery(dirVector...),
			Using:  qdrant.PtrOf("dirs"),
			Filter: filters,
		},
		{
			// Contents vector query
			Query:  qdrant.NewQuery(contentVector...),
			Using:  qdrant.PtrOf("contents"),
			Filter: filters,
		},
	}

	// Create hybrid query with weighted fusion
	hybridQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQueryFusion(qdrant.Fusion_RRF),
		Prefetch:       prefetchQueries,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	searchResp, err := client.Query(ctx, hybridQuery)
	if err != nil {
		return nil, fmt.Errorf("error performing RRF hybrid search in %s: %v", collectionName, err)
	}

	// First, collect all results and their scores
	var allResults []SearchResult
	var scores []float32

	for _, point := range searchResp {
		result := convertPointToResult(point)
		allResults = append(allResults, result)
		scores = append(scores, point.Score)
	}

	if len(scores) == 0 {
		log.Printf("No search results found")
		return []ComponentGroup{}, nil
	}

	// Sort scores to analyze distribution
	sort.Slice(scores, func(i, j int) bool {
		return scores[i] > scores[j] // Sort descending
	})

	minScore, maxScore := scores[len(scores)-1], scores[0]
	log.Printf("Score range: min=%.4f, max=%.4f, total results=%d", minScore, maxScore, len(scores))

	// Calculate adaptive threshold based on score distribution
	var adaptiveThreshold float32

	if len(scores) <= 3 {
		// For very few results, use a lower threshold to be more inclusive
		adaptiveThreshold = minScore + (maxScore-minScore)*0.2
	} else {
		// Calculate percentile-based threshold
		// Use 60th percentile (keep top 40% of results) for better distribution-aware filtering
		percentileIndex := int(float32(len(scores)) * 0.6)
		if percentileIndex >= len(scores) {
			percentileIndex = len(scores) - 1
		}
		percentileThreshold := scores[percentileIndex]

		// Also calculate mean and standard deviation for additional context
		var sum float32
		for _, score := range scores {
			sum += score
		}
		mean := sum / float32(len(scores))

		var variance float32
		for _, score := range scores {
			variance += (score - mean) * (score - mean)
		}
		stdDev := float32(math.Sqrt(float64(variance / float32(len(scores)))))

		log.Printf("Score distribution: mean=%.4f, stddev=%.4f, 60th percentile=%.4f", mean, stdDev, percentileThreshold)

		// Use the higher of: 60th percentile or (mean - 0.5*stddev)
		// This ensures we don't exclude results that are reasonably close to the average
		meanBasedThreshold := mean - 0.5*stdDev
		adaptiveThreshold = float32(math.Max(float64(percentileThreshold), float64(meanBasedThreshold)))

		// But don't let the threshold be too low compared to the score range
		minThreshold := minScore + (maxScore-minScore)*0.1
		if adaptiveThreshold < minThreshold {
			adaptiveThreshold = minThreshold
		}
	}

	log.Printf("Calculated adaptive threshold: %.4f (method: distribution-aware)", adaptiveThreshold)

	// Now filter results using the adaptive threshold
	var results []SearchResult
	for _, result := range allResults {
		if result.Score >= adaptiveThreshold {
			log.Printf("DEBUG: RRF result for purl %s, version %s, score %.4f (meets adaptive threshold)", result.Purl, result.Version, result.Score)
			results = append(results, result)
		} else {
			log.Printf("DEBUG: Excluding RRF result with score %.4f (did not meet > %.4f adaptive threshold for %s)", result.Score, adaptiveThreshold, result.Purl)
		}
	}

	log.Printf("RRF hybrid search found %d quality results in %s after filtering", len(results), collectionName)

	// Group by component
	componentGroups := groupByComponent(results)

	log.Printf("RRF hybrid search completed: %d results grouped into %d components", len(results), len(componentGroups))
	return componentGroups, nil
}
