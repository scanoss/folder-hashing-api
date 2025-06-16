package entities

// ScanRequest represents the domain model for folder hash scanning request
type ScanRequest struct {
	// Get results with rank above this threshold (e.g i only want to see results from rank 3 and above)
	RankThreshold int32 `validate:"omitempty,min=0"`
	// Filter results by category (e.g i only want to see results from github projects, npm, etc)
	Category string
	// Maximum number of results to query
	QueryLimit int32 `validate:"omitempty,min=1"`
	// Folder root node to be scanned
	Root *FolderNode `validate:"required"`
}

// FolderNode represents a folder node in the hierarchical structure
type FolderNode struct {
	// Folder path (can be actual or obfuscated)
	PathID string `validate:"required"`
	// Proximity hash calculated from this nodes filenames (and their children)
	SimHashNames string `validate:"required"`
	// Proximity hash calculated from this nodes file contents (and their children)
	SimHashContent string `validate:"required"`
	// Proximity hash calculated from this nodes directory names (and their children)
	SimHashDirNames string `validate:"required"`
	// Language extensions count (dictionary) - language name -> count
	LangExtensions LanguageExtensions
	// Sub-folders inside this child
	Children []*FolderNode
}

// LanguageExtensions represents file extension counts by language
type LanguageExtensions map[string]int32
