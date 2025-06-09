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

	// Legacy collection names for separate collections approach (deprecated)
	DirsCollectionName     = "dirs_collection"
	NamesCollectionName    = "names_collection"
	ContentsCollectionName = "contents_collection"

	ADAPTIVE_THRESHOLD_PERCENTAGE = 0.5 // Use 50% of the distance range as threshold
)

// Category scoring weights for boosting distances based on repository category
// Lower multipliers mean better (smaller) distances after boosting
var CategoryDistanceBoosts = map[string]float32{
	"github_popular": 0.7, // 30% improvement (smaller distance) for popular repositories
	"github":         0.9, // 10% improvement for regular github repositories
	"common":         1.0, // No change for common repositories
	"forks":          1.2, // 20% penalty (larger distance) for forks
}

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

// buildExcludedLanguageExtensionFilter creates conditions to exclude entries that have extensions not in the query
// For example, if querying a Python project (py, md, sh), exclude entries that also have c, h, java, etc.
func buildExcludedLanguageExtensionFilter(queryLangExt LanguageExtensions) []*qdrant.Condition {
	var conditions []*qdrant.Condition

	// Get all possible indexed extensions that are NOT in our query
	for _, extension := range IndexedLangExtensions {
		// Skip if this extension is in our query (we want to allow these)
		if _, exists := queryLangExt[extension]; exists {
			continue
		}

		// Skip empty extension check as it's common
		if extension == "" {
			continue
		}

		// Create condition to exclude entries that have this extension with count > 0
		condition := &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "language_extensions." + extension,
					Range: &qdrant.Range{
						Gt: qdrant.PtrOf(float64(0)), // Exclude if this extension count > 0
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
		Distance: point.Score,
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

// getCategoryRankValue returns a numeric rank for category-based sorting
// Lower values indicate higher priority (github_popular > github > common)
func getCategoryRankValue(category string) int {
	switch category {
	case "github_popular":
		return 1
	case "github":
		return 2
	case "common":
		return 3
	default:
		return 4 // For any unknown categories
	}
}

// boostDistanceByCategory applies category-based distance boost to improve ranking
func boostDistanceByCategory(point *qdrant.ScoredPoint) float32 {
	originalDistance := point.Score

	// Get category from payload
	if point.Payload != nil {
		if val, exists := point.Payload["category"]; exists {
			category := val.GetStringValue()
			if boost, exists := CategoryDistanceBoosts[category]; exists {
				boostedDistance := originalDistance * boost
				log.Printf("DEBUG: Applied category boost for %s: %.4f -> %.4f (boost: %.1fx)", category, originalDistance, boostedDistance, boost)
				return boostedDistance
			}
		}
	}

	return originalDistance
}

// calculateGapBasedThreshold calculates adaptive threshold using gap-based analysis for better filtering
func calculateGapBasedThreshold(distances []float32) float32 {
	if len(distances) == 0 {
		return 0
	}

	// Sort distances to analyze distribution (ascending - lower distance is better)
	sort.Slice(distances, func(i, j int) bool {
		return distances[i] < distances[j]
	})

	minDistance, maxDistance := distances[0], distances[len(distances)-1]
	log.Printf("Distance range: min=%.4f, max=%.4f, total results=%d", minDistance, maxDistance, len(distances))

	var adaptiveThreshold float32

	// Gap-based filtering for better quality control
	if minDistance < 1.0 {
		// Very good match - be very selective
		adaptiveThreshold = float32(math.Min(float64(minDistance*3.0), 4.0))
		log.Printf("High-quality match detected (distance < 1.0), using restrictive threshold: %.4f", adaptiveThreshold)
	} else if minDistance < 5.0 {
		// Good match - moderately selective
		adaptiveThreshold = float32(math.Min(float64(minDistance*2.0), 5.0))
		log.Printf("Good quality match detected (distance < 5.0), using moderate threshold: %.4f", adaptiveThreshold)
	} else {
		// No clear winner - use distribution analysis but cap at reasonable distance
		if len(distances) <= 3 {
			adaptiveThreshold = float32(math.Min(float64(minDistance+(maxDistance-minDistance)*0.6), 8.0))
		} else {
			// Use percentile-based threshold but cap it
			percentileIndex := int(float32(len(distances)) * 0.3) // More restrictive - keep top 30%
			if percentileIndex >= len(distances) {
				percentileIndex = len(distances) - 1
			}
			percentileThreshold := distances[percentileIndex]

			// Cap at 8.0 distance maximum
			adaptiveThreshold = float32(math.Min(float64(percentileThreshold), 8.0))
		}
		log.Printf("No clear winner, using distribution-based threshold (capped at 8.0): %.4f", adaptiveThreshold)
	}

	// Absolute maximum threshold - never include anything with distance > 8.0
	if adaptiveThreshold > 8.0 {
		adaptiveThreshold = 8.0
		log.Printf("Capping threshold at maximum acceptable distance: %.4f", adaptiveThreshold)
	}

	log.Printf("Calculated gap-based threshold: %.4f", adaptiveThreshold)
	return adaptiveThreshold
}

// GetConfidenceLevel returns a confidence assessment based on distance and evidence count
func GetConfidenceLevel(distance float32, evidenceCount int) (string, string) {
	if distance < 1.0 {
		return "🎯 VERY HIGH", "Excellent match with strong evidence"
	} else if distance < 2.0 && evidenceCount > 2 {
		return "🔥 HIGH", "Strong match with good evidence"
	} else if distance < 4.0 && evidenceCount > 1 {
		return "✅ MEDIUM", "Good match with moderate evidence"
	} else if distance < 6.0 {
		return "⚠️ LOW", "Possible match but limited confidence"
	} else {
		return "❓ VERY LOW", "Weak match, likely false positive"
	}
}

// combineWeightedResults combines results from three separate queries with weights
func combineWeightedResults(namesResp, dirsResp, contentsResp []*qdrant.ScoredPoint, namesWeight, dirsWeight, contentsWeight float32, topK uint64) []SearchResult {
	combinedDistances := make(map[uint64]float32)
	pointMap := make(map[uint64]*qdrant.ScoredPoint)

	// Process names results (75% weight) - lower distance is better
	for _, point := range namesResp {
		id := point.Id.GetNum()
		combinedDistances[id] += point.Score * namesWeight
		pointMap[id] = point
	}

	// Process dirs results (15% weight) - lower distance is better
	for _, point := range dirsResp {
		id := point.Id.GetNum()
		combinedDistances[id] += point.Score * dirsWeight
		if _, exists := pointMap[id]; !exists {
			pointMap[id] = point
		}
	}

	// Process contents results (10% weight) - lower distance is better
	for _, point := range contentsResp {
		id := point.Id.GetNum()
		combinedDistances[id] += point.Score * contentsWeight
		if _, exists := pointMap[id]; !exists {
			pointMap[id] = point
		}
	}

	// Convert to slice and sort by category rank, then by combined distance
	type distanceResult struct {
		id           uint64
		distance     float32
		point        *qdrant.ScoredPoint
		categoryRank int
	}

	var distanceResults []distanceResult
	for id, distance := range combinedDistances {
		point := pointMap[id]
		categoryRank := 4 // Default rank for unknown/missing category

		// Extract category from point payload
		if point.Payload != nil {
			if val, exists := point.Payload["category"]; exists {
				category := val.GetStringValue()
				categoryRank = getCategoryRankValue(category)
			}
		}

		distanceResults = append(distanceResults, distanceResult{
			id:           id,
			distance:     distance,
			point:        point,
			categoryRank: categoryRank,
		})
	}

	// Sort by category rank first (ascending - lower rank is better), then by combined distance (ascending - lower distance is better)
	sort.Slice(distanceResults, func(i, j int) bool {
		if distanceResults[i].categoryRank == distanceResults[j].categoryRank {
			return distanceResults[i].distance < distanceResults[j].distance
		}
		return distanceResults[i].categoryRank < distanceResults[j].categoryRank
	})

	// Convert to SearchResult and limit to topK
	var results []SearchResult
	limit := int(topK)
	if limit > len(distanceResults) {
		limit = len(distanceResults)
	}

	for i := 0; i < limit; i++ {
		// Update the point's score to the combined distance
		distanceResults[i].point.Score = distanceResults[i].distance
		result := convertPointToResult(distanceResults[i].point)
		results = append(results, result)
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

			// Update best match if this version has a better distance (lower is better)
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

// SearchLanguageBasedApproximate performs adaptive threshold search with progressive filtering
func SearchLanguageBasedApproximate(config QdrantSeparateConfig, dirHash, nameHash, contentHash string, queryLangExt LanguageExtensions, topK uint64) ([]ComponentGroup, error) {
	log.Printf("Starting adaptive threshold search (names→dirs→contents with 75%%/15%%/10%% weights)")

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

	// Build language extension filter for additional filtering
	if len(queryLangExt) > 0 {
		langExtConditions := buildLanguageExtensionFilter(queryLangExt, 10.0)
		// excludedExtConditions := buildExcludedLanguageExtensionFilter(queryLangExt)
		if len(langExtConditions) > 0 {
			mustConditions = append(mustConditions, langExtConditions...)
		}
		// if len(excludedExtConditions) > 0 {
		// 	mustNotConditions = append(mustNotConditions, excludedExtConditions...)
		// }
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

	// Execute names query
	namesQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(nameVector...),
		Using:          qdrant.PtrOf("names"),
		Filter:         filters,
		Limit:          qdrant.PtrOf(uint64(5000)),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	// Step 1: Execute names query and calculate distribution-aware adaptive threshold
	namesResp, err := client.Query(ctx, namesQuery)
	if err != nil {
		return nil, fmt.Errorf("error performing names query in %s: %v", collectionName, err)
	}

	if len(namesResp) == 0 {
		log.Printf("No results found in names query for %s", collectionName)
		return []ComponentGroup{}, nil
	}

	// Apply category-based distance boosting first, then collect distances for threshold calculation
	var boostedDistances []float32
	var boostedNamesResp []*qdrant.ScoredPoint

	for _, point := range namesResp {
		// Create a copy of the point to avoid modifying the original
		boostedPoint := *point
		boostedDistance := boostDistanceByCategory(point)
		boostedPoint.Score = boostedDistance
		boostedNamesResp = append(boostedNamesResp, &boostedPoint)
		boostedDistances = append(boostedDistances, boostedDistance)
	}

	// Calculate gap-based adaptive threshold using boosted distances
	adaptiveThreshold := calculateGapBasedThreshold(boostedDistances)

	// Filter names results by adaptive threshold using boosted distances
	var filteredNamesResp []*qdrant.ScoredPoint
	for _, point := range boostedNamesResp {
		if point.Score <= adaptiveThreshold {
			filteredNamesResp = append(filteredNamesResp, point)
		}
	}
	log.Printf("Names results: %d total, %d after distribution-aware adaptive threshold", len(namesResp), len(filteredNamesResp))

	// Execute dirs query with names prefetch
	dirsQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(dirVector...),
		Using:          qdrant.PtrOf("dirs"),
		Limit:          qdrant.PtrOf(uint64(len(filteredNamesResp))),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	// Execute contents query with names prefetch
	contentsQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(contentVector...),
		Using:          qdrant.PtrOf("contents"),
		Limit:          qdrant.PtrOf(uint64(len(filteredNamesResp))),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	}

	// Step 2: Execute dirs query with filtered names as constraint
	dirsResp, err := client.Query(ctx, dirsQuery)
	if err != nil {
		return nil, fmt.Errorf("error performing dirs query in %s: %v", collectionName, err)
	}

	// Filter dirs results to only include IDs from filtered names
	namesIDSet := make(map[uint64]bool)
	for _, point := range filteredNamesResp {
		namesIDSet[point.Id.GetNum()] = true
	}

	var filteredDirsResp []*qdrant.ScoredPoint
	for _, point := range dirsResp {
		if namesIDSet[point.Id.GetNum()] {
			filteredDirsResp = append(filteredDirsResp, point)
		}
	}
	log.Printf("Dirs results: %d total, %d matching names threshold", len(dirsResp), len(filteredDirsResp))

	// Step 3: Execute contents query for final tie-breaking on dirs-filtered results
	contentsResp, err := client.Query(ctx, contentsQuery)
	if err != nil {
		return nil, fmt.Errorf("error performing contents query in %s: %v", collectionName, err)
	}

	// Create ID set from dirs results for final filtering
	dirsIDSet := make(map[uint64]bool)
	for _, point := range filteredDirsResp {
		dirsIDSet[point.Id.GetNum()] = true
	}

	var filteredContentsResp []*qdrant.ScoredPoint
	for _, point := range contentsResp {
		if dirsIDSet[point.Id.GetNum()] {
			filteredContentsResp = append(filteredContentsResp, point)
		}
	}
	log.Printf("Contents results: %d total, %d matching dirs threshold", len(contentsResp), len(filteredContentsResp))

	results := combineWeightedResults(filteredNamesResp, filteredDirsResp, filteredContentsResp, 0.75, 0.15, 0.10, topK)

	log.Printf("Adaptive threshold search found %d results in %s (names→dirs→contents with 75%%+15%%+10%% weights)", len(results), collectionName)

	componentGroups := groupByComponent(results)

	log.Printf("Weighted hybrid search completed: %d results grouped into %d components", len(results), len(componentGroups))
	return componentGroups, nil
}
