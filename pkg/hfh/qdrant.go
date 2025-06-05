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

	LOW_SIMILARITY_THRESHOLD = 0.5

	// Optimized thresholds for approximate search only
	HIGH_SIMILARITY_THRESHOLD_APPROX   = 5  // 1-5 bit differences (very strict)
	MEDIUM_SIMILARITY_THRESHOLD_APPROX = 12 // 6-12 bit differences (moderate)
	LOW_SIMILARITY_THRESHOLD_APPROX    = 20 // 13-20 bit differences (permissive)
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

	// Build language extension filter for additional filtering
	var extensionsFilter *qdrant.Filter
	if len(queryLangExt) > 0 {
		langExtConditions := buildLanguageExtensionFilter(queryLangExt, 5.0) // More tolerant for language-based collections
		excludedExtConditions := buildExcludedLanguageExtensionFilter(queryLangExt)
		if len(langExtConditions) > 0 {
			extensionsFilter = &qdrant.Filter{
				Must:    langExtConditions,
				MustNot: excludedExtConditions,
			}
		}
	}

	// Create prefetch queries for RRF fusion (weights handled by fusion algorithm)
	prefetchQueries := []*qdrant.PrefetchQuery{
		{
			// Names vector query
			Query:  qdrant.NewQuery(nameVector...),
			Using:  qdrant.PtrOf("names"),
			Limit:  qdrant.PtrOf(topK * 2),
			Filter: extensionsFilter,
		},
		{
			// Dirs vector query
			Query:  qdrant.NewQuery(dirVector...),
			Using:  qdrant.PtrOf("dirs"),
			Limit:  qdrant.PtrOf(topK * 2),
			Filter: extensionsFilter,
		},
		{
			// Contents vector query
			Query:  qdrant.NewQuery(contentVector...),
			Using:  qdrant.PtrOf("contents"),
			Limit:  qdrant.PtrOf(topK * 2),
			Filter: extensionsFilter,
		},
	}

	// Create hybrid query with weighted fusion
	hybridQuery := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQueryFusion(qdrant.Fusion_RRF),
		Prefetch:       prefetchQueries,
		Limit:          qdrant.PtrOf(topK),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
		Params: &qdrant.SearchParams{
			HnswEf: qdrant.PtrOf(uint64(128)),
			Exact:  qdrant.PtrOf(false),
		},
	}

	searchResp, err := client.Query(ctx, hybridQuery)
	if err != nil {
		return nil, fmt.Errorf("error performing weighted hybrid search in %s: %v", collectionName, err)
	}

	// Convert results and apply quality filtering
	var results []SearchResult
	for _, point := range searchResp {
		result := convertPointToResult(point)

		if result.Score > LOW_SIMILARITY_THRESHOLD {
			log.Printf("Quality match in %s: %s %s (score: %.3f)", collectionName, result.Component, result.Version, result.Score)
			results = append(results, result)
		}
	}

	log.Printf("Weighted hybrid search found %d quality results in %s", len(results), collectionName)

	// Group by component
	componentGroups := groupByComponent(results)

	log.Printf("Weighted hybrid search completed: %d results grouped into %d components", len(results), len(componentGroups))
	return componentGroups, nil
}
