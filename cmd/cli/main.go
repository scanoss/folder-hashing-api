// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2018-2024 SCANOSS.COM
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

// Package main implements the HFH CLI for folder hashing and similarity search
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/scanoss/folder-hashing-api/pkg/hfh"
	hfh_cli "github.com/scanoss/folder-hashing-api/pkg/usecase/examples/hfh_cli"
)

func main() {
	// Check if subcommand is provided
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "search":
		searchCommand()
	case "help", "-help", "--help":
		showHelp()
	default:
		fmt.Printf("Unknown subcommand: %s\n", subcommand)
		showHelp()
		os.Exit(1)
	}
}

func searchCommand() {
	// Create a new flag set for the search subcommand
	searchFlags := flag.NewFlagSet("search", flag.ExitOnError)
	dirPath := searchFlags.String("dir", "", "Directory path to hash and search for similar projects (required)")
	host := searchFlags.String("host", "localhost", "Qdrant server host")
	port := searchFlags.Int("port", 6334, "Qdrant server port")
	topK := searchFlags.Int("top", 10, "Number of top similar results to return")
	help := searchFlags.Bool("help", false, "Show help message")
	searchFlags.Parse(os.Args[2:])

	// Show help if requested or no directory specified
	if *help || *dirPath == "" {
		showSearchHelp()
		if *dirPath == "" {
			os.Exit(1)
		}
		return
	}

	// Verify directory exists
	if _, err := os.Stat(*dirPath); os.IsNotExist(err) {
		log.Fatalf("Error: Directory '%s' does not exist", *dirPath)
	}

	// Get absolute path
	absPath, err := filepath.Abs(*dirPath)
	if err != nil {
		log.Fatalf("Error getting absolute path: %v", err)
	}

	fmt.Printf("Calculating hashes for directory: %s\n", absPath)
	fmt.Println("=" + repeatString("=", len(absPath)+33))

	// Calculate hashes using the source of truth implementation
	requestRoot, err := hfh_cli.HFHrequestFromPath(absPath)
	if err != nil {
		log.Fatalf("Error calculating hashes: %v", err)
	}

	if requestRoot == nil {
		log.Fatal("Error: No hash result returned (directory may be empty or all files filtered)")
	}

	fmt.Printf("\nQuery Hashes:\n")
	fmt.Printf("  Directory Hash:   %v\n", requestRoot.SimHashDirNames)
	fmt.Printf("  Names Hash:       %v\n", requestRoot.SimHashNames)
	fmt.Printf("  Contents Hash:    %v\n", requestRoot.SimHashContent)
	fmt.Printf("Searching for similar projects in Qdrant...\n")
	fmt.Printf("Host: %s:%d, Top: %d\n", *host, *port, *topK)
	fmt.Println(repeatString("-", 60))

	// Search for similar projects in Qdrant using the new language-based hybrid search
	config := hfh.NewQdrantSeparateConfig(*host, *port)

	// Display language extensions detected
	if len(requestRoot.LangExtensions) > 0 {
		fmt.Printf("Language Extensions Detected: %v\n", requestRoot.LangExtensions)
		targetCollection := hfh.GetCollectionNameFromLanguageExtensions(requestRoot.LangExtensions)
		fmt.Printf("Target Collection: %s\n", targetCollection)
	} else {
		fmt.Printf("No language extensions detected, using misc_collection\n")
	}

	// Choose search variant based on user preference
	componentGroups, err := hfh.SearchLanguageBasedApproximate(config, requestRoot.SimHashDirNames, requestRoot.SimHashNames, requestRoot.SimHashContent, requestRoot.LangExtensions, uint64(*topK))
	if err != nil {
		log.Fatalf("Error searching in Qdrant: %v", err)
	}

	if len(componentGroups) == 0 {
		fmt.Println("No similar projects found.")
		return
	}

	// Display grouped results
	displayGroupedResults(componentGroups)
}

func showHelp() {
	fmt.Println("HFH CLI - Hierarchical Folder Hashing Tool with Language-Based Qdrant Search")
	fmt.Println("=============================================================================")
	fmt.Println()
	fmt.Println("This tool calculates folder hashes and searches for similar projects using language-based collections.")
	fmt.Println("The search uses an advanced hybrid approach:")
	fmt.Println("  1. Language detection - automatically determines target collection (Python, JavaScript, Java, etc.)")
	fmt.Println("  2. Multi-vector search - combines directory structure, file names, and content hashes")
	fmt.Println("  3. Hybrid fusion - uses Qdrant's RRF (Reciprocal Rank Fusion) for optimal ranking")
	fmt.Println("  4. Extension filtering - applies language-specific filters for higher precision")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Println("  search   Calculate hashes and search for similar projects in language-based collections")
	fmt.Println("  help     Show this help message")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hfh-cli <subcommand> [options]")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hfh-cli search -dir /path/to/python-project -top 5")
	fmt.Println("  hfh-cli search -dir . -host localhost -port 6334")
	fmt.Println("  hfh-cli search -dir /path/to/js-project -top 10")
	fmt.Println()
	fmt.Println("For subcommand-specific help:")
	fmt.Println("  hfh-cli search -help")
}

func showSearchHelp() {
	fmt.Println("HFH CLI - Language-Based Hybrid Search")
	fmt.Println("======================================")
	fmt.Println()
	fmt.Println("Calculate hashes for a directory and search for similar projects using language-based collections.")
	fmt.Println("This advanced search approach automatically:")
	fmt.Println()
	fmt.Println("1. Language Detection:")
	fmt.Println("   - Analyzes file extensions to determine primary language")
	fmt.Println("   - Selects appropriate collection (python_collection, javascript_collection, etc.)")
	fmt.Println("   - Falls back to misc_collection if no language detected")
	fmt.Println()
	fmt.Println("2. Multi-Vector Hybrid Search:")
	fmt.Println("   - Directory structure vector (dirs)")
	fmt.Println("   - File naming patterns vector (names)")
	fmt.Println("   - Content similarity vector (contents)")
	fmt.Println("   - Uses Qdrant's RRF (Reciprocal Rank Fusion) for optimal ranking")
	fmt.Println()
	fmt.Println("3. Language-Specific Filtering:")
	fmt.Println("   - Applies extension-based filters for higher precision")
	fmt.Println("   - Searches within language-specific collections for better performance")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hfh-cli search -dir <directory_path> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -dir string      Directory path to hash and search (required)")
	fmt.Println("  -host string     Qdrant server host (default: localhost)")
	fmt.Println("  -port int        Qdrant server port (default: 6334)")
	fmt.Println("  -top int         Number of top similar results to return (default: 10)")
	fmt.Println("  -weighted        Use weighted fusion variant instead of RRF (experimental)")
	fmt.Println("  -help           Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hfh-cli search -dir /path/to/python-project")
	fmt.Println("  hfh-cli search -dir . -top 5")
	fmt.Println("  hfh-cli search -dir ../js-project -host 192.168.1.100 -port 6334")
	fmt.Println("  hfh-cli search -dir /path/to/go-project -top 20 -weighted")
	fmt.Println()
	fmt.Println("Supported Languages:")
	fmt.Println("  Python, JavaScript/TypeScript, Java, C/C++, Go, Rust, PHP, Ruby,")
	fmt.Println("  C#, Scala, Kotlin, Swift, Shell, Web (HTML/CSS), Dart, SQL, Lua, R")
	fmt.Println()
	fmt.Println("Note: Requires a running Qdrant server with language-based collections created via the import tool.")
}

func repeatString(s string, count int) string {
	result := ""
	for range count {
		result += s
	}
	return result
}

// displayGroupedResults displays grouped search results
func displayGroupedResults(componentGroups []hfh.ComponentGroup) {
	fmt.Printf("\nFound %d component group(s):\n", len(componentGroups))
	fmt.Println(repeatString("=", 80))

	for i, group := range componentGroups {
		// Display component header
		fmt.Printf("\n%d. Component: %s\n", i+1, group.Component)
		fmt.Printf("   Vendor: %s\n", group.Vendor)
		fmt.Printf("   📊 Result Count: %d supporting results\n", group.ResultCount)

		// Add quality indicator based on result count
		if group.ResultCount > 5 {
			fmt.Printf("   🎯 Strong Evidence - Multiple results point to this component\n")
		} else if group.ResultCount > 2 {
			fmt.Printf("   ✅ Good Evidence - Several results support this component\n")
		} else if group.ResultCount > 1 {
			fmt.Printf("   ⚠️  Moderate Evidence - Some evidence for this component\n")
		} else {
			fmt.Printf("   ❓ Limited Evidence - Single result for this component\n")
		}

		// Display best match
		fmt.Printf("   \n🏆 BEST MATCH:\n")
		fmt.Printf("     Version: %s\n", group.BestMatch.Version)
		fmt.Printf("     Score: %.4f\n", group.BestMatch.Score)

		if group.BestMatch.URL != "" {
			fmt.Printf("     URL: %s\n", group.BestMatch.URL)
		}

		// Display metadata for best match
		if license, ok := group.BestMatch.Metadata["license"]; ok && license != "" {
			fmt.Printf("     License: %v\n", license)
		}
		if totalFiles, ok := group.BestMatch.Metadata["total_files"]; ok {
			fmt.Printf("     Total Files: %v\n", totalFiles)
		}
		if size, ok := group.BestMatch.Metadata["size"]; ok {
			fmt.Printf("     Size: %v bytes\n", size)
		}
		if releaseDate, ok := group.BestMatch.Metadata["release_date"]; ok && releaseDate != "" {
			fmt.Printf("     Release Date: %v\n", releaseDate)
		}

		// Display other versions if available
		if len(group.OtherVersions) > 0 {
			fmt.Printf("   \n📦 OTHER AVAILABLE VERSIONS:\n")
			for j, version := range group.OtherVersions {
				fmt.Printf("     %d. %s", j+1, version)

				// Find the detailed version info
				for _, versionDetail := range group.AllVersions {
					if versionDetail.Version == version {
						fmt.Printf(" (Score: %.4f)", versionDetail.Score)
						break
					}
				}
				fmt.Println()

				// Limit to max 5 other versions for readability
				if j >= 4 {
					remaining := len(group.OtherVersions) - j - 1
					if remaining > 0 {
						fmt.Printf("     ... and %d more versions\n", remaining)
					}
					break
				}
			}
		}

		if len(group.AllVersions) > 1 {
			fmt.Printf("     📚 Multiple Versions Available (%d)\n", len(group.AllVersions))
		}

		// Additional result analysis
		if group.ResultCount > 1 {
			fmt.Printf("     🔍 Multiple Supporting Results (%d) - Higher Reliability\n", group.ResultCount)
		}

		if i < len(componentGroups)-1 {
			fmt.Println(repeatString("-", 80))
		}
	}

	// Summary information
	fmt.Printf("\n" + repeatString("=", 80) + "\n")
	fmt.Printf("SEARCH SUMMARY:\n")
	fmt.Printf("• Found %d unique component(s)\n", len(componentGroups))

	totalVersions := 0
	highScoreCount := 0
	multiResultCount := 0
	totalSupportingResults := 0

	for _, group := range componentGroups {
		totalVersions += len(group.AllVersions)
		totalSupportingResults += group.ResultCount
		if group.ResultCount > 1 {
			multiResultCount++
		}
	}

	fmt.Printf("• Total versions discovered: %d\n", totalVersions)
	fmt.Printf("• Total supporting search results: %d\n", totalSupportingResults)
	fmt.Printf("• High similarity matches: %d\n", highScoreCount)
	fmt.Printf("• Components with multiple results: %d\n", multiResultCount)

	if len(componentGroups) > 0 {
		bestMatch := componentGroups[0]
		fmt.Printf("• Best overall match: %s %s (%.4f score)\n",
			bestMatch.Component, bestMatch.BestMatch.Version, bestMatch.BestMatch.Score)

		if bestMatch.ResultCount > 2 {
			fmt.Printf("• 🎯 Strong recommendation: High similarity with multiple supporting results for %s\n", bestMatch.Component)
		}
	}
}
