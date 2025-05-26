package hfh

import (
	"hash/crc64"
)

// CRC64 table for hashing
var crcTable = crc64.MakeTable(crc64.ECMA)

// FileItem represents a file with its metadata
type FileItem struct {
	Path       string
	Size       int64
	IsDir      bool
	Content    []byte
	ShouldHash bool
}

// DirectoryNode represents a directory in the tree structure
type DirectoryNode struct {
	Path     string
	IsDir    bool
	Children map[string]*DirectoryNode
	Files    []FileItem
}

// HashResult contains the three different hash types
type HashResult struct {
	DirHash     uint64
	NameHash    uint64
	ContentHash uint64
}

// NewDirectoryNode creates a new directory node
func NewDirectoryNode(path string) *DirectoryNode {
	return &DirectoryNode{
		Path:     path,
		IsDir:    true,
		Children: make(map[string]*DirectoryNode),
		Files:    make([]FileItem, 0),
	}
}

// InitKeyHash initializes the CRC64 table (equivalent to m.InitKeyHash())
func InitKeyHash() {
	// The table is already initialized globally
}
