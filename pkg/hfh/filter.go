package hfh

import (
	"path/filepath"
	"strings"
)

// ShouldAcceptPath determines if a path should be included in hashing
// This implements similar logic to the original filtering
func ShouldAcceptPath(path string) bool {
	// Get the filename
	filename := filepath.Base(path)
	
	// Skip hidden files and directories
	if strings.HasPrefix(filename, ".") {
		return false
	}
	
	// Skip common build/cache directories
	excludedDirs := []string{
		"node_modules",
		".git",
		".svn",
		".hg",
		"target",
		"build",
		"dist",
		"out",
		"bin",
		"obj",
		".gradle",
		".idea",
		".vscode",
		"__pycache__",
		".pytest_cache",
		"coverage",
		".coverage",
		"vendor",
	}
	
	pathParts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	for _, part := range pathParts {
		for _, excluded := range excludedDirs {
			if part == excluded {
				return false
			}
		}
	}
	
	return true
}

// EvaluateItem simulates the original filter.EvaluateItem function
func EvaluateItem(item FileItem) FileItem {
	// For simplicity, we'll set ShouldHash based on path acceptance
	// and some basic file type filtering
	item.ShouldHash = ShouldAcceptPath(item.Path)
	
	// Skip very large files (> 10MB) to avoid memory issues
	if item.Size > 10*1024*1024 {
		item.ShouldHash = false
	}
	
	// Skip binary files that are commonly not useful for content hashing
	ext := strings.ToLower(filepath.Ext(item.Path))
	binaryExts := []string{
		".exe", ".dll", ".so", ".dylib", ".a", ".lib",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".ico",
		".mp3", ".mp4", ".avi", ".mov", ".mkv", ".wmv",
		".zip", ".tar", ".gz", ".rar", ".7z",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
	}
	
	for _, binExt := range binaryExts {
		if ext == binExt {
			item.ShouldHash = false
			break
		}
	}
	
	return item
}
