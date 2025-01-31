package filter

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	m "scanoss.com/hfh-api/pkg/usecase/examples/hfh_cli/go-minr-deps/model"
	u "scanoss.com/hfh-api/pkg/usecase/examples/hfh_cli/go-minr-deps/utils"
)

const MIN_FILE_SIZE int = 256
const MAX_FILE_SIZE int = 8 * 1024 * 1024

// var SKIP_SNIPPET_EXT = []string{".exe", ".zip", ".tar", ".tgz", ".gz", ".7z", ".rar", ".jar", ".war", ".ear", ".class", ".pyc", ".o", ".a", ".so", ".obj", ".dll", ".lib", ".out", ".app", ".bin", ".lst", ".dat", ".json", ".htm", ".html", ".xml", ".md", ".txt", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp", ".pages", ".key", ".numbers", ".pdf", ".min.js", ".mf", ".sum", ".sym"}
var IGNORED_EXTENSIONS = []string{".1", ".2", ".3", ".4", ".5", ".6", ".7", ".8", ".9", ".ac", ".adoc", ".am", ".asciidoc", ".bmp", ".build", ".c9", ".c9revisions", ".cache", ".cfg", ".chm", ".class", ".cmake", ".cnf", ".conf", ".config", ".contributors", ".copying", ".cover", ".coverage", ".crt", ".csproj", ".css", ".csv", ".dat", ".data", ".doc", ".docx", ".dotcover", ".dtd", ".dts", ".dtsi", ".dump", ".editorconfig", ".egg", ".eot", ".eps", ".geojson", ".gem", ".gdoc", ".gif", ".gml", ".glif", ".gmo", ".gradle", ".guess", ".hex", ".htm", ".html", ".ico", ".iml", ".in", ".inc", ".info", ".ini", ".ipynb", ".jpeg", ".jpg", ".json", ".jsonld", ".lcov", ".lock", ".log", ".lst", ".m4", ".manifest", ".map", ".markdown", ".md", ".md5", ".meta", ".mk", ".mxml", ".o", ".otf", ".out", ".pbtxt", ".pdb", ".pdf", ".pem", ".phtml", ".pickle", ".pid", ".plist", ".png", ".po", ".pom", ".pot", ".prefs", ".properties", ".plt", ".pyc", ".qdoc", ".result", ".rgb", ".rst", ".scss", ".sha", ".sha1", ".sha2", ".sha256", ".smtml", ".sln", ".spec", ".sql", ".sub", ".svg", ".svn-base", ".tab", ".template", ".test", ".tex", ".tiff", ".toml", ".ttf", ".txt", ".utf-8", ".vim", ".wav", ".whl", ".woff", ".wfp", ".xht", ".xhtml", ".xls", ".xlsx", ".xml", ".xpm", ".xsd", ".xul", ".yaml", ".yml"}
var IGNORED_PATHS = []string{"/__pycache__/", "/__pypackages__", "/_yardoc/", "/eggs/", "/htmlcov/", "/nbbuild/", "/nbdist/", "/nbproject/", "/venv/", "/wheels/"}
var IGNORED_PREFIX = []string{"."}
var IGNORED_SUBFIXES = []string{"-DOC", "CHANGELOG", "CONFIG", "COPYING", "COPYING.LIB", "LICENSE", "LICENSE.MD", "LICENSE.TXT", "LICENSES", "MAKEFILE", "NOTICE", "NOTICE", "README", "SWIFTDOC", "TEXIDOC", "TODO", "VERSION"}
var IGNORED_ENDINGS = []string{".min.js", ".MF", ".base64", ".s"}

// Evaluate actions to be done on each downloaded item
func EvaluateItem(fileItem u.FileDesc) m.FileItem {

	file := path.Base(fileItem.Name)
	ret := m.FileActions{CompletelyIgnore: false, StoreInFile: false, StoreInMZ: false, StoreInExtraMZ: false, StoreInPivot: false, IsLicenseFile: false}

	if fileItem.IsSymlink {
		ret.CompletelyIgnore = true
		return m.FileItem{Path: fileItem.Name, Actions: ret}
	}

	// Hidden files -> Completely ignored
	for _, r := range IGNORED_PREFIX {
		if strings.HasPrefix(file, r) {
			ret.CompletelyIgnore = true
			return m.FileItem{Path: fileItem.Name, Actions: ret}
		}
	}
	// files on hidden folders
	paths := strings.Split(fileItem.Name, "/")
	for _, subpath := range paths {
		if strings.HasPrefix(subpath, ".") {
			ret.CompletelyIgnore = true
			return m.FileItem{Path: fileItem.Name, Actions: ret}
		}
	}

	fileInfo, err := os.Stat(fileItem.Name)
	if err != nil {
		fmt.Println("Error al obtener información del archivo:", err)
	}

	// Get File size in bytes
	fileSize := fileInfo.Size()

	if err != nil {
		log.Printf("Error opening this file: %v", err)
		ret.CompletelyIgnore = true
		return m.FileItem{Path: fileItem.Name, Actions: ret}

	}
	var x []byte
	var isTextFile bool
	var endsWithNull bool

	if fileSize > 10000000 {
		x, err = u.FileCRC64(fileItem.Name)
		if err != nil {

			ret.CompletelyIgnore = true
			return m.FileItem{Path: fileItem.Name, Actions: ret}
		}
		isTextFile, _ = u.FileIsTextFile(fileItem.Name)
		endsWithNull, _ = u.LastCharIsNull(fileItem.Name)
	} else {
		content, _ := os.ReadFile(fileItem.Name)

		k := m.GetHashBuff(content)
		isTextFile = u.IsText(content)
		if len(content) > 1 {
			endsWithNull = content[len(content)-1] == 0
		}
		x = k[:]

	}

	// Empty files -> Completely ignored
	if fileSize == 0 {
		ret.CompletelyIgnore = true
		return m.FileItem{Path: fileItem.Name, Actions: ret}
	}

	keyStr := fmt.Sprintf("%0*x", len(x), x)
	if endsWithNull && isTextFile {
		ret.CompletelyIgnore = true
		return m.FileItem{Path: fileItem.Name, Actions: ret, KeyStr: keyStr, Key: x[:]}
	}
	//if path has a comma (,) replace it with "-"
	if strings.Contains(fileItem.Name, ",") {
		newName := strings.ReplaceAll(fileItem.Name, ",", "-")
		os.Rename(fileItem.Name, newName)
		fileItem.Name = newName
	}

	for _, r := range IGNORED_SUBFIXES {
		if strings.HasSuffix(file, r) {
			ret.StoreInFile = true
			ret.StoreInPivot = true
			ret.StoreInExtraMZ = true
			return m.FileItem{Path: fileItem.Name, Actions: ret, KeyStr: keyStr, Key: x[:]}
		}
	}

	for _, r := range IGNORED_PATHS {
		if strings.Contains(fileItem.Name, r) {
			ret.StoreInFile = true
			ret.StoreInPivot = true

			if isTextFile {
				ret.StoreInExtraMZ = true

			}

			return m.FileItem{Path: fileItem.Name, Actions: ret, KeyStr: keyStr, Key: x[:]}
		}
	}
	//ignored extensions
	extension := path.Ext(fileItem.Name)
	for _, r := range IGNORED_EXTENSIONS {
		if extension == r {
			ret.StoreInFile = true
			ret.StoreInPivot = true
			if isTextFile {
				ret.StoreInExtraMZ = true

			}

			return m.FileItem{Path: fileItem.Name, Actions: ret, KeyStr: keyStr, Key: x[:]}
		}
	}

	if !isTextFile {
		ret.StoreInPivot = true
		ret.StoreInFile = true
		ret.StoreInMZ = false
		return m.FileItem{Path: fileItem.Name, Actions: ret, KeyStr: keyStr, Key: x[:]}
	}
	// Files ending with null -> Completely ignored

	ret.StoreInFile = true
	ret.StoreInPivot = true
	if fileSize < 0xFFFFFFFF {
		ret.StoreInMZ = true
	} else {
		ret.StoreInMZ = false
	}

	return m.FileItem{Path: fileItem.Name, Actions: ret, KeyStr: keyStr, Key: x[:]}

}
