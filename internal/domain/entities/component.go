package entities

// ComponentGroup represents grouped results by component name
type ComponentGroup struct {
	Component     string          `json:"component"`
	Vendor        string          `json:"vendor"`
	BestMatch     VersionResult   `json:"best_match"`
	OtherVersions []string        `json:"other_versions,omitempty"`
	AllVersions   []VersionResult `json:"all_versions,omitempty"`
	ResultCount   int             `json:"result_count"`
	Rank          int             `json:"rank"`
}

// VersionResult represents a version-specific result within a component group
type VersionResult struct {
	Version            string             `json:"version"`
	Score              float32            `json:"score"`
	URL                string             `json:"url,omitempty"`
	Purl               string             `json:"purl,omitempty"`
	LanguageExtensions LanguageExtensions `json:"language_extensions,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
}

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
	Rank               int                `json:"rank"`
}

// Component represents a component match result for protobuf compatibility
type Component struct {
	// Component PURL
	PURL string
	// Component match version (could be multiple)
	Versions []string
	// Component Ranking from 1 to 10. 1 means official well known repository, 10 might be garbage.
	Rank int32
}
