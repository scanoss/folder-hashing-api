package hfh

import (
	"log"
	"path/filepath"
	"strings"
)

// Default skip rules - aligned with original hfh_hash.go
var (
	skippedFiles = map[string]bool{
		"gradlew":            true,
		"gradlew.bat":        true,
		"mvnw":               true,
		"mvnw.cmd":           true,
		"gradle-wrapper.jar": true,
		"maven-wrapper.jar":  true,
		"thumbs.db":          true,
		"babel.config.js":    true,
		// Add common documentation and metadata files
		"license":                   true,
		"readme":                    true,
		"changelog":                 true,
		"changelog.md":              true,
		"readme.md":                 true,
		"license.md":                true,
		"license.txt":               true,
		"readme.txt":                true,
		"authors":                   true,
		"contributors":              true,
		"copying":                   true,
		"install":                   true,
		"news":                      true,
		"todo":                      true,
		"version":                   true,
		"manifest":                  true,
		"manifest.in":               true,
		"pkg-info":                  true,
		"__init__":                  true,
		"__init__.py":               true,
		"makefile":                  true,
		"dockerfile":                true,
		"code_of_conduct":           true,
		"code_of_conduct.md":        true,
		"contributing":              true,
		"contributing.md":           true,
		"implementation_summary":    true,
		"implementation_summary.md": true,
		"migration_summary":         true,
		"migration_summary.md":      true,
	}

	skippedDirs = map[string]bool{
		"example":        true,
		"examples":       true,
		"nbproject":      true,
		"nbbuild":        true,
		"nbdist":         true,
		"__pycache__":    true,
		"venv":           true,
		"_yardoc":        true,
		"eggs":           true,
		"wheels":         true,
		"htmlcov":        true,
		"__pypackages__": true,
	}

	skippedDirExt = []string{".egg-info"}

	skippedExt = []string{
		".1",
		".2",
		".3",
		".4",
		".5",
		".6",
		".7",
		".8",
		".9",
		".ac",
		".adoc",
		".am",
		".asciidoc",
		".bmp",
		".build",
		".cfg",
		".chm",
		".class",
		".cmake",
		".cnf",
		".conf",
		".config",
		".contributors",
		".copying",
		".crt",
		".csproj",
		".css",
		".csv",
		".dat",
		".data",
		".dtd",
		".dts",
		".iws",
		".c9",
		".c9revisions",
		".dtsi",
		".dump",
		".eot",
		".eps",
		".geojson",
		".gif",
		".glif",
		".gmo",
		".guess",
		".hex",
		".htm",
		".html",
		".ico",
		".iml",
		".in",
		".inc",
		".info",
		".ini",
		".ipynb",
		".jpeg",
		".jpg",
		".json",
		".jsonld",
		".lock",
		".log",
		".m4",
		".map",
		".md5",
		".meta",
		".mk",
		".mxml",
		".o",
		".otf",
		".out",
		".pbtxt",
		".pdf",
		".pem",
		".phtml",
		".plist",
		".png",
		".prefs",
		".properties",
		".pyc",
		".qdoc",
		".result",
		".rgb",
		".rst",
		".scss",
		".sha",
		".sha1",
		".sha2",
		".sha256",
		".sln",
		".spec",
		".sub",
		".svg",
		".svn-base",
		".tab",
		".template",
		".test",
		".tex",
		".tiff",
		".ttf",
		".txt",
		".utf-8",
		".vim",
		".wav",
		".woff",
		".woff2",
		".xht",
		".xhtml",
		".xml",
		".xpm",
		".xsd",
		".xul",
		".yaml",
		".yml",
		".wfp",
		".editorconfig",
		".dotcover",
		".pid",
		".lcov",
		".egg",
		".manifest",
		".cache",
		".coverage",
		".cover",
		".gem",
		".lst",
		".pickle",
		".pdb",
		".gml",
		".pot",
		".plt",
		".whml",
		".pom",
		".smtml",
		".min.js",
		".mf",
		".base64",
		".s",
		".diff",
		".patch",
		".rules",
		// File endings
		"-doc",
		"config",
		"news",
		"readme",
		"swiftdoc",
		"texidoc",
		"todo",
		"version",
		"ignore",
		"manifest",
		"sqlite",
		"sqlite3",
	}
)

// ShouldAcceptPath determines if a given path should be included in scanning/fingerprinting
// based solely on the path string analysis - aligned with original hfh_hash.go
func ShouldAcceptPath(path string) bool {
	// Clean the path
	path = filepath.Clean(path)

	// Check each component of the path for hidden files/folders
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}

	// Get the base name (last component of path)
	dirLower := strings.ToLower(filepath.Dir(path))
	fileLower := strings.ToLower(filepath.Base(path))

	normalizedPath := filepath.ToSlash(dirLower)
	pathComponents := strings.Split(normalizedPath, "/")

	for _, subFolder := range pathComponents {
		// Check default skipped directories
		if skippedDirs[subFolder] {
			return false
		}

		// Check directory extensions to skip
		for _, ext := range skippedDirExt {
			if strings.HasSuffix(subFolder, ext) {
				log.Printf("Dir skipped: %s\n", dirLower)
				return false
			}
		}
	}

	// Also check if the final component (file/directory name) has a directory extension to skip
	for _, ext := range skippedDirExt {
		if strings.HasSuffix(fileLower, ext) {
			log.Printf("Dir skipped: %s\n", path)
			return false
		}
	}

	// Check if file should be skipped
	if skippedFiles[fileLower] {
		log.Printf("DEBUG: File %s skipped - in skippedFiles list", fileLower)
		return false
	}

	// Check file extensions and endings
	for _, ext := range skippedExt {
		if strings.HasSuffix(fileLower, ext) {
			log.Printf("DEBUG: File %s skipped - matches suffix %s", fileLower, ext)
			return false
		}
	}

	log.Printf("DEBUG: File %s ACCEPTED", fileLower)
	return true
}

// EvaluateItem simulates the original filter.EvaluateItem function
func EvaluateItem(item FileItem) FileItem {
	// Set ShouldHash based on path acceptance and file size
	item.ShouldHash = ShouldAcceptPath(item.Path)

	// Skip very large files (> 10MB) to avoid memory issues
	if item.Size > 10*1024*1024 {
		item.ShouldHash = false
	}

	// Additional check for filename length (from original logic)
	fileName := filepath.Base(item.Path)
	if len(fileName) > 32 {
		item.ShouldHash = false
	}

	return item
}
