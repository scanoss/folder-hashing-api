package hfh

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LoadPath loads a directory path and builds a directory tree structure
// This is equivalent to the loadPath function in the original hfh_cli.go
func LoadPath(path string) (*DirectoryNode, error) {
	files := GetAllFiles(path)
	if files == nil {
		return nil, fmt.Errorf("invalid or empty directory")
	}
	
	root := filepath.Clean(path)
	rootParts := strings.Split(root, string(filepath.Separator))
	rootNode := NewDirectoryNode(root)

	for f := range files {
		item := EvaluateItem(files[f])
		if !item.ShouldHash {
			continue
		}
		if !ShouldAcceptPath(item.Path) {
			continue
		}
		
		dir := filepath.Dir(item.Path)
		parts := strings.Split(dir, string(filepath.Separator))
		rootNode.Files = append(rootNode.Files, item)

		currentNode := rootNode
		currentPath := root
		for i := len(rootParts); i < len(parts); i++ {
			part := parts[i]
			currentPath = filepath.Join(currentPath, part)

			if _, exists := currentNode.Children[currentPath]; !exists {
				currentNode.Children[currentPath] = NewDirectoryNode(currentPath)
			}
			currentNode = currentNode.Children[currentPath]
			currentNode.Files = append(currentNode.Files, item)
		}
	}

	return rootNode, nil
}

// HFHRequestFromPath creates hash calculation from a directory path
// This is equivalent to the HFHrequestFromPath function in the original hfh_cli.go
func HFHRequestFromPath(path string) (*HashResult, error) {
	// Init CRC64 table
	InitKeyHash()
	
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	
	rootNode, err := LoadPath(absolutePath)
	if err != nil {
		return nil, err
	}
	
	hashResult := HashCalc(rootNode)
	return hashResult, nil
}
