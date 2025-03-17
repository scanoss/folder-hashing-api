package hfh_cli

import (
	"log"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mfonda/simhash"
)

type HFHhash struct {
	NameHash    uint64
	ContentHash uint64
}

/* Calc hash head */
func headCalc(simHash uint64) byte {
	var sum int
	for i := 0; i < 8; i++ {
		b := byte((simHash >> (i * 8)) & 0xFF)
		sum += int(b) * 2
	}
	return byte(sum >> 4 & 0xFF)
}

func HashCalc(node *directoryNode) *HFHhash {
	processedHashes := make(map[string]bool)
	var fileHashesList [][]byte
	var selectedNames []string

	for _, file := range node.Files {
		if _, processed := processedHashes[file.KeyStr]; processed {
			continue
		}
		if !file.Actions.StoreInFile || file.Actions.CompletelyIgnore {
			continue
		}

		if !ShouldAcceptPath(file.Path) {
			continue
		}
		fileName := filepath.Base(file.Path)
		if len(fileName) > 32 {
			continue
		}

		processedHashes[file.KeyStr] = true
		selectedNames = append(selectedNames, fileName)
		fileHashesList = append(fileHashesList, file.Key)

	}
	if len(selectedNames) < 8 {
		return nil
	}
	sort.Strings(selectedNames)
	concatenatedNames := strings.Join(selectedNames, "")
	log.Println(concatenatedNames)
	if len(concatenatedNames) < 32 {
		return nil
	}
	/* Calc Files name simhash */
	FilesNameSimhash := simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))
	/* Calc Files content simhash */
	FilesContentSimhash := simhash.Fingerprint(simhash.VectorizeBytes(fileHashesList))

	/* Calc hash head to group close hashes by sector */
	head := headCalc(FilesNameSimhash)
	//log.Printf("Main hash head: %02x\n", head)

	/*Overwrite the MS byte with the head to keep the hash size total */
	FilesNameSimhash = (FilesNameSimhash & 0x00FFFFFFFFFFFFFF) | (uint64(head) << 56)
	log.Printf("%x - %x\n", FilesNameSimhash, FilesContentSimhash)

	return &HFHhash{
		NameHash:    FilesNameSimhash,
		ContentHash: FilesContentSimhash,
	}
}

// Default skip rules
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
		"license.txt":        true,
		"license.md":         true,
		"copying.lib":        true,
		"makefile":           true,
	}

	skippedDirs = map[string]bool{
		"example":        true,
		"examples":       true,
		"docs":           true,
		"tests":          true,
		"doc":            true,
		"test":           true,
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
		".1", ".2", ".3", ".4", ".5", ".6", ".7", ".8", ".9",
		".ac", ".adoc", ".am", ".asciidoc", ".bmp", ".build",
		".cfg", ".chm", ".class", ".cmake", ".cnf", ".conf",
		".config", ".contributors", ".copying", ".crt", ".csproj",
		".css", ".csv", ".dat", ".data", ".doc", ".docx", ".dtd",
		".dts", ".iws", ".c9", ".c9revisions", ".dtsi", ".dump",
		".eot", ".eps", ".geojson", ".gdoc", ".gif", ".glif",
		".gmo", ".gradle", ".guess", ".hex", ".htm", ".html",
		".ico", ".iml", ".in", ".inc", ".info", ".ini", ".ipynb",
		".jpeg", ".jpg", ".json", ".jsonld", ".lock", ".log",
		".m4", ".map", ".markdown", ".md", ".md5", ".meta", ".mk",
		".mxml", ".o", ".otf", ".out", ".pbtxt", ".pdf", ".pem",
		".phtml", ".plist", ".png", ".po", ".ppt", ".prefs",
		".properties", ".pyc", ".qdoc", ".result", ".rgb", ".rst",
		".scss", ".sha", ".sha1", ".sha2", ".sha256", ".sln",
		".spec", ".sql", ".sub", ".svg", ".svn-base", ".tab",
		".template", ".test", ".tex", ".tiff", ".toml", ".ttf",
		".txt", ".utf-8", ".vim", ".wav", ".woff", ".woff2",
		".xht", ".xhtml", ".xls", ".xlsx", ".xml", ".xpm",
		".xsd", ".xul", ".yaml", ".yml", ".wfp", ".editorconfig",
		".dotcover", ".pid", ".lcov", ".egg", ".manifest",
		".cache", ".coverage", ".cover", ".gem", ".lst",
		".pickle", ".pdb", ".gml", ".pot", ".plt", ".whml",
		".pom", ".smtml", ".min.js", ".mf", ".base64", ".s", ".diff", ".patch", ".rules",
		// File endings
		"-doc", "changelog", "config", "copying", "license",
		"authors", "news", "licenses", "notice", "readme",
		"swiftdoc", "texidoc", "todo", "version", "ignore",
		"manifest", "sqlite", "sqlite3",
	}
)

// ShouldAcceptPath determines if a given path should be included in scanning/fingerprinting
// based solely on the path string analysis
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
	base := filepath.Base(path)
	baseLower := strings.ToLower(base)

	// Check if it looks like a directory (ends with path separator)
	isDir := strings.HasSuffix(path, string(filepath.Separator))

	if isDir {
		// Check default skipped directories
		if skippedDirs[baseLower] {
			return false
		}

		// Check directory extensions to skip
		for _, ext := range skippedDirExt {
			if strings.HasSuffix(baseLower, ext) {
				return false
			}
		}

		return true
	}

	// Check if file should be skipped
	if skippedFiles[baseLower] {
		return false
	}

	// Check file extensions and endings
	for _, ext := range skippedExt {
		if strings.HasSuffix(baseLower, ext) {
			return false
		}
	}

	return true
}
