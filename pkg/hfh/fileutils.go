package hfh

import (
	"io"
	"os"
	"path/filepath"
)

// GetAllFiles recursively walks through a directory and returns all files
// This is equivalent to the u.GetAllFiles function in the original code
func GetAllFiles(rootPath string) map[int]FileItem {
	files := make(map[int]FileItem)
	index := 0
	
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}
		
		// Create FileItem
		item := FileItem{
			Path:  path,
			Size:  info.Size(),
			IsDir: info.IsDir(),
		}
		
		// Only read content for files (not directories) and if they're not too large
		if !info.IsDir() && info.Size() <= 10*1024*1024 { // 10MB limit
			content, err := readFileContent(path)
			if err == nil {
				item.Content = content
			}
		}
		
		files[index] = item
		index++
		return nil
	})
	
	if err != nil {
		return nil
	}
	
	return files
}

// readFileContent reads the content of a file
func readFileContent(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	return io.ReadAll(file)
}
