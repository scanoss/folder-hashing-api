package entities

// ScanRequest represents the domain model for folder hash scanning request
type ScanRequest struct {
	// Report best match (most hits) or not
	BestMatch bool
	// Detection threshold (distance) for component selection
	Threshold int32
	// Folder root node to be scanned
	Root *FolderNode
}

// FolderNode represents a folder node in the hierarchical structure
type FolderNode struct {
	// Folder path (can be actual or obfuscated)
	PathID string
	// Proximity hash calculated from this nodes filenames (and their children)
	SimHashNames string
	// Proximity hash calculated from this nodes file contents (and their children)
	SimHashContent string
	// Proximity hash calculated from this nodes directory names (and their children)
	SimHashDirNames string
	// Language extensions count (dictionary) - language name -> count
	LangExtensions LanguageExtensions
	// Sub-folders inside this child
	Children []*FolderNode
}

// LanguageExtensions represents file extension counts by language
type LanguageExtensions map[string]int32
