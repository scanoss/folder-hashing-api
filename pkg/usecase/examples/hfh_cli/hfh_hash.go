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
	DirHash     uint64
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
	fileMapUnique := make(map[string]bool)
	dirMapUnique := make(map[string]bool)

	if len(node.Files) < 10 {
		return nil
	}
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
		extension := filepath.Ext(fileName)
		filenameWithoutExt := strings.TrimSuffix(fileName, extension)
		fileMapUnique[filenameWithoutExt] = true

		dir := filepath.Dir(file.Path)
		lastFolder := filepath.Base(dir)
		dirMapUnique[lastFolder] = true

		processedHashes[file.KeyStr] = true
		selectedNames = append(selectedNames, fileName)
		fileHashesList = append(fileHashesList, file.Key)

	}
	if len(selectedNames) < 8 {
		return nil
	}
	sort.Strings(selectedNames)
	concatenatedNames := strings.Join(selectedNames, "")

	if len(concatenatedNames) < 32 {
		return nil
	}

	/* Calc Files name simhash */
	FilesNameSimhash := simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))

	FilteredUniqueFileNames := make([]string, 0, len(fileMapUnique))
	for k := range fileMapUnique {
		FilteredUniqueFileNames = append(FilteredUniqueFileNames, k)
	}
	sort.Strings(FilteredUniqueFileNames)
	log.Println(FilteredUniqueFileNames)

	concatenatedNames = strings.Join(FilteredUniqueFileNames, " ")

	FilesNameSimhashNorm := simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))

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

	//FilesNameSimhashNorm ^= FilesNameSimhash
	/* Calc Files content simhash */
	FilesContentSimhash := simhash.Fingerprint(simhash.VectorizeBytes(fileHashesList))

	//bytes1 := make([]byte, 8) // uint64 ocupa 8 bytes
	//binary.LittleEndian.PutUint64(bytes1, FilesNameSimhash)
	//bytes2 := make([]byte, 8) // uint64 ocupa 8 bytes
	//binary.LittleEndian.PutUint64(bytes2, FilesNameSimhashNorm)
	//FilesNameSimhashNorm = simhash.Fingerprint(simhash.VectorizeBytes([][]byte{bytes1, bytes2}))

	/* Calc hash head to group close hashes by sector */
	/*	head := headCalc(FilesNameSimhash)
		//log.Printf("Main hash head: %02x\n", head)
		//Overwrite the MS byte with the head to keep the hash size total
		FilesNameSimhash = (FilesNameSimhash & 0x00FFFFFFFFFFFFFF) | (uint64(head) << 56)*/
	log.Printf("%x/%x - %x\n", FilesNameSimhash, FilesNameSimhashNorm, FilesContentSimhash)

	return &HFHhash{
		NameHash:    FilesNameSimhashNorm,
		ContentHash: FilesContentSimhash,
		DirHash:     DirsSimhashNorm,
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

	// Check if file should be skipped
	if skippedFiles[fileLower] {
		return false
	}

	// Check file extensions and endings
	for _, ext := range skippedExt {
		if strings.HasSuffix(fileLower, ext) {
			return false
		}
	}

	return true
}
