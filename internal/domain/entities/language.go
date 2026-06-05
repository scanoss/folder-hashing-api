// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2024 SCANOSS.COM
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 2 of the License, or
 * (at your option) any later version.
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package entities

import (
	"strings"
)

// Collection name constants for languages that map from multiple extensions.
const (
	javascriptCollection = "javascript_collection"
	javaCollection       = "java_collection"
	cppCollection        = "cpp_collection"
	shellCollection      = "shell_collection"
	webCollection        = "web_collection"

	// extSwift is the Swift source file extension.
	extSwift = "swift"
)

// PrimaryLanguages maps file extensions to their corresponding collection names.
var PrimaryLanguages = map[string]string{
	"js":     javascriptCollection,
	"jsx":    javascriptCollection,
	"ts":     javascriptCollection,
	"tsx":    javascriptCollection,
	"py":     "python_collection",
	"java":   javaCollection,
	"class":  javaCollection,
	"jar":    javaCollection,
	"c":      "c_collection",
	"h":      "c_collection",
	"cpp":    cppCollection,
	"cxx":    cppCollection,
	"cc":     cppCollection,
	"hpp":    cppCollection,
	"hxx":    cppCollection,
	"go":     "go_collection",
	"rb":     "ruby_collection",
	"php":    "php_collection",
	"cs":     "csharp_collection",
	"rs":     "rust_collection",
	"scala":  "scala_collection",
	"kt":     "kotlin_collection",
	extSwift: "swift_collection",
	"sh":     shellCollection,
	"bash":   shellCollection,
	"zsh":    shellCollection,
	"html":   webCollection,
	"css":    webCollection,
	"scss":   webCollection,
	"less":   webCollection,
	"vue":    webCollection,
	"svelte": webCollection,
	"dart":   "dart_collection",
	"sql":    "sql_collection",
	"lua":    "lua_collection",
	"r":      "r_collection",
	"":       "misc_collection", // Files without extension
}

// IndexedLangExtensions is a list of all file extensions that are indexed by the system.
var IndexedLangExtensions = []string{
	// Web/Frontend
	"ts", "js", "jsx", "tsx", "html", "css", "scss", "less", "vue", "svelte",
	// Backend/General
	"py", "java", "class", "jar", "go", "rb", "php", "cs", "rs", "scala", "kt", "groovy", "clj", "ex", "exs",
	// C-family
	"c", "h", "cpp", "cxx", "cc", "hpp", "hxx", "m", "mm", extSwift,
	// Shell/Scripts
	"sh", "bash", "zsh", "ps1", "bat", "cmd", "pl", "pm", "t",
	// Data/Config
	"json", "yaml", "yml", "xml", "toml", "ini", "conf", "cfg", "properties",
	// Documentation
	"md", "rst", "txt", "tex", "adoc", "wiki",
	// Mobile
	"dart", "kotlin", extSwift, "gradle",
	// Database
	"sql", "graphql", "prisma",
	// Other
	"lua", "r", "d", "fs", "f", "f90", "hs", "erl", "elm", "lisp", "jl",
	// Empty extension (for files without extension)
	"",
}

// GetPrimaryLanguageFromExtensions determines the most common language from extension counts.
func GetPrimaryLanguageFromExtensions(langExt LanguageExtensions) string {
	if len(langExt) == 0 {
		return "misc"
	}

	maxCount := int32(0)
	primaryLang := "misc"

	// Find the extension with the highest count
	for ext, count := range langExt {
		if count > maxCount {
			if collectionName, exists := PrimaryLanguages[ext]; exists {
				maxCount = count
				primaryLang = strings.TrimSuffix(collectionName, "_collection")
			}
		}
	}

	return primaryLang
}

// GetCollectionNameFromLanguageExtensions gets the target collection based on language extensions.
func GetCollectionNameFromLanguageExtensions(langExt LanguageExtensions) string {
	primaryLang := GetPrimaryLanguageFromExtensions(langExt)
	return primaryLang + "_collection"
}

// GetAllSupportedCollections returns all unique collection names from the PrimaryLanguages map.
func GetAllSupportedCollections() []string {
	collectionsMap := make(map[string]bool)
	for _, collectionName := range PrimaryLanguages {
		collectionsMap[collectionName] = true
	}

	collections := make([]string, 0, len(collectionsMap))
	for collectionName := range collectionsMap {
		collections = append(collections, collectionName)
	}

	return collections
}
