package hfh

import (
	"log"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mfonda/simhash"
)

// HashCalc calculates the three types of hashes for a directory node
// Aligned with original hfh_hash.go logic using simhash
func HashCalc(dirNode *DirectoryNode) *HashResult {
	if dirNode == nil {
		return nil
	}

	processedHashes := make(map[string]bool)
	var fileHashesList [][]byte
	var selectedNames []string
	fileMapUnique := make(map[string]bool)
	dirMapUnique := make(map[string]bool)

	// Collect all files from the directory node
	allFiles := collectAllFiles(dirNode)

	if len(allFiles) < 10 {
		return nil
	}

	for _, file := range allFiles {
		// Create a unique key for the file (similar to original KeyStr)
		keyStr := file.Path + string(rune(file.Size))

		if _, processed := processedHashes[keyStr]; processed {
			continue
		}

		// Debug logging to understand filtering
		fileName := filepath.Base(file.Path)
		shouldAccept := ShouldAcceptPath(file.Path)
		log.Printf("DEBUG: File: %s, ShouldHash: %t, ShouldAcceptPath: %t", fileName, file.ShouldHash, shouldAccept)

		if !file.ShouldHash {
			log.Printf("DEBUG: Skipping %s - ShouldHash=false", fileName)
			continue
		}

		if !ShouldAcceptPath(file.Path) {
			log.Printf("DEBUG: Skipping %s - ShouldAcceptPath=false", fileName)
			continue
		}

		if len(fileName) > 32 {
			log.Printf("DEBUG: Skipping %s - filename too long (%d chars)", fileName, len(fileName))
			continue
		}

		// Only process actual files for file names hash, not directories
		if !file.IsDir {
			extension := filepath.Ext(fileName)
			filenameWithoutExt := strings.TrimSuffix(fileName, extension)
			fileMapUnique[filenameWithoutExt] = true
			selectedNames = append(selectedNames, fileName)
			log.Printf("DEBUG: Processing file: %s -> %s", fileName, filenameWithoutExt)

			if len(file.Content) > 0 {
				fileHashesList = append(fileHashesList, file.Content)
			}

			// Collect directory names from the path of files, but only within the project scope
			dir := filepath.Dir(file.Path)
			rootPath := dirNode.Path

			// Walk up the directory path and collect intermediate directory names
			for dir != "." && dir != "/" && dir != "\\" && strings.HasPrefix(dir, rootPath) {
				lastFolder := filepath.Base(dir)
				// Skip root directory and apply same filtering as files
				if lastFolder != "." && lastFolder != ".." &&
					lastFolder != filepath.Base(rootPath) && // Skip root directory
					ShouldAcceptPath(dir) { // Apply same filtering logic
					dirMapUnique[lastFolder] = true
					log.Printf("DEBUG: Adding directory from path: %s", lastFolder)
				}
				dir = filepath.Dir(dir)
			}
		} else {
			// For directories, apply the same filtering logic as files
			if fileName != "." && fileName != ".." &&
				fileName != filepath.Base(dirNode.Path) && // Skip root directory
				ShouldAcceptPath(file.Path) { // Apply same filtering logic
				dirMapUnique[fileName] = true
				log.Printf("DEBUG: Processing directory: %s", fileName)
			} else {
				log.Printf("DEBUG: Skipping directory: %s (filtered)", fileName)
			}
		}

		processedHashes[keyStr] = true
	}

	if len(selectedNames) < 8 {
		return nil
	}

	sort.Strings(selectedNames)
	concatenatedNames := strings.Join(selectedNames, "")

	if len(concatenatedNames) < 32 {
		return nil
	}

	// Calculate Files name simhash (not used in final result but kept for compatibility)
	_ = simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))

	// Calculate normalized file names hash
	FilteredUniqueFileNames := make([]string, 0, len(fileMapUnique))
	for k := range fileMapUnique {
		FilteredUniqueFileNames = append(FilteredUniqueFileNames, k)
	}
	sort.Strings(FilteredUniqueFileNames)
	log.Println("FilteredUniqueFileNames", FilteredUniqueFileNames)

	concatenatedNames = strings.Join(FilteredUniqueFileNames, " ")
	FilesNameSimhashNorm := simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))

	// Calculate directory names hash
	FilteredUniqueDirNames := make([]string, 0, len(dirMapUnique))
	for k := range dirMapUnique {
		if k == "." || k == ".." {
			continue
		}
		FilteredUniqueDirNames = append(FilteredUniqueDirNames, k)
	}
	sort.Strings(FilteredUniqueDirNames)
	concatenatedNames = strings.Join(FilteredUniqueDirNames, " ")
	DirsSimhashNorm := simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))
	log.Println("FilteredUniqueDirNames", FilteredUniqueDirNames)

	// Calculate Files content simhash
	FilesContentSimhash := simhash.Fingerprint(simhash.VectorizeBytes(fileHashesList))

	log.Printf("%x/%x - %x\n", FilesNameSimhashNorm, DirsSimhashNorm, FilesContentSimhash)

	return &HashResult{
		NameHash:    FilesNameSimhashNorm,
		ContentHash: FilesContentSimhash,
		DirHash:     DirsSimhashNorm,
	}
}

// collectAllFiles recursively collects all files from a directory node
func collectAllFiles(dirNode *DirectoryNode) []FileItem {
	if dirNode == nil {
		return nil
	}

	var allFiles []FileItem

	// Add files from current node
	allFiles = append(allFiles, dirNode.Files...)

	// Recursively add files from children
	for _, child := range dirNode.Children {
		childFiles := collectAllFiles(child)
		allFiles = append(allFiles, childFiles...)
	}

	return allFiles
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
