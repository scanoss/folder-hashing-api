package entities

// ComponentGroup represents a grouped component with versions
type ComponentGroup struct {
	PURL     string    `json:"purl"`
	Versions []Version `json:"versions"`
	Rank     int32     `json:"rank"`
	Order    int32     `json:"order"`
}

// Version represents a component version with score
type Version struct {
	Version string  `json:"version"`
	Score   float32 `json:"score"`
}

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Score     float32 `json:"score"`
	ID        uint64  `json:"id"`
	Vendor    string  `json:"vendor"`
	Component string  `json:"component"`
	Purl      string  `json:"purl"`
	Version   string  `json:"version"`
	Rank      int     `json:"rank"`
}
