package hfh

import (
	"hash/crc64"
	"path/filepath"
	"sort"
	"strings"
)

// HashCalc calculates the three types of hashes for a directory node
func HashCalc(dirNode *DirectoryNode) *HashResult {
	if dirNode == nil {
		return nil
	}
	
	// Initialize hash result
	result := &HashResult{}
	
	// Calculate directory structure hash
	result.DirHash = calculateDirHash(dirNode)
	
	// Calculate file names hash
	result.NameHash = calculateNameHash(dirNode)
	
	// Calculate file contents hash
	result.ContentHash = calculateContentHash(dirNode)
	
	return result
}

// calculateDirHash computes hash based on directory structure
func calculateDirHash(dirNode *DirectoryNode) uint64 {
	hash := crc64.New(crcTable)
	
	// Collect all directory paths in sorted order for consistent hashing
	var dirPaths []string
	collectDirPaths(dirNode, &dirPaths)
	sort.Strings(dirPaths)
	
	// Hash the directory structure
	for _, dirPath := range dirPaths {
		// Use relative path from root for consistency
		relPath := strings.TrimPrefix(dirPath, dirNode.Path)
		relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
		if relPath != "" {
			hash.Write([]byte(relPath))
		}
	}
	
	return hash.Sum64()
}

// calculateNameHash computes hash based on file names
func calculateNameHash(dirNode *DirectoryNode) uint64 {
	hash := crc64.New(crcTable)
	
	// Collect all file names in sorted order for consistent hashing
	var fileNames []string
	collectFileNames(dirNode, &fileNames)
	sort.Strings(fileNames)
	
	// Hash the file names
	for _, fileName := range fileNames {
		hash.Write([]byte(fileName))
	}
	
	return hash.Sum64()
}

// calculateContentHash computes hash based on file contents
func calculateContentHash(dirNode *DirectoryNode) uint64 {
	hash := crc64.New(crcTable)
	
	// Collect all file contents, sorted by file path for consistency
	var fileContents [][]byte
	var filePaths []string
	collectFileContents(dirNode, &fileContents, &filePaths)
	
	// Sort by file path and hash contents in that order
	type fileData struct {
		path    string
		content []byte
	}
	
	var files []fileData
	for i, path := range filePaths {
		if i < len(fileContents) {
			files = append(files, fileData{path: path, content: fileContents[i]})
		}
	}
	
	sort.Slice(files, func(i, j int) bool {
		return files[i].path < files[j].path
	})
	
	// Hash the file contents
	for _, file := range files {
		if len(file.content) > 0 {
			hash.Write(file.content)
		}
	}
	
	return hash.Sum64()
}

// collectDirPaths recursively collects all directory paths
func collectDirPaths(dirNode *DirectoryNode, paths *[]string) {
	if dirNode == nil {
		return
	}
	
	*paths = append(*paths, dirNode.Path)
	
	for _, child := range dirNode.Children {
		collectDirPaths(child, paths)
	}
}

// collectFileNames recursively collects all file names
func collectFileNames(dirNode *DirectoryNode, names *[]string) {
	if dirNode == nil {
		return
	}
	
	// Add file names from current directory
	for _, file := range dirNode.Files {
		if !file.IsDir && file.ShouldHash {
			fileName := filepath.Base(file.Path)
			*names = append(*names, fileName)
		}
	}
	
	// Recursively collect from children
	for _, child := range dirNode.Children {
		collectFileNames(child, names)
	}
}

// collectFileContents recursively collects all file contents and their paths
func collectFileContents(dirNode *DirectoryNode, contents *[][]byte, paths *[]string) {
	if dirNode == nil {
		return
	}
	
	// Add file contents from current directory
	for _, file := range dirNode.Files {
		if !file.IsDir && file.ShouldHash && len(file.Content) > 0 {
			*contents = append(*contents, file.Content)
			*paths = append(*paths, file.Path)
		}
	}
	
	// Recursively collect from children
	for _, child := range dirNode.Children {
		collectFileContents(child, contents, paths)
	}
}

// HashesToVector converts three 64-bit hashes into a 192-dimensional binary vector by concatenation
func HashesToVector(dirHash, nameHash, contentHash uint64) []float32 {
	// Create a 192-dimensional vector (3 * 64 bits)
	vector := make([]float32, 192)
	
	// Fill first 64 dimensions with dir hash bits
	for i := 0; i < 64; i++ {
		if (dirHash>>i)&1 == 1 {
			vector[i] = 1.0
		} else {
			vector[i] = 0.0
		}
	}
	
	// Fill next 64 dimensions with name hash bits
	for i := 0; i < 64; i++ {
		if (nameHash>>i)&1 == 1 {
			vector[i+64] = 1.0
		} else {
			vector[i+64] = 0.0
		}
	}
	
	// Fill last 64 dimensions with content hash bits
	for i := 0; i < 64; i++ {
		if (contentHash>>i)&1 == 1 {
			vector[i+128] = 1.0
		} else {
			vector[i+128] = 0.0
		}
	}
	
	return vector
}

// CreateCombinedHash creates a simple combined hash from three 64-bit hashes
// This is used for point ID generation - the actual similarity matching uses the concatenated vector
func CreateCombinedHash(dirHash, nameHash, contentHash uint64) uint64 {
	// Simple XOR combination for point ID - this is just for unique identification
	// The actual similarity matching will use the concatenated vector
	return dirHash ^ nameHash ^ contentHash
}
