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
	"strconv"

	"scanoss.com/hfh-api/pkg/hfh"
	hfh_cli "scanoss.com/hfh-api/pkg/usecase/examples/hfh_cli"
)

func main() {
	// Check if subcommand is provided
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "hash":
		hashCommand()
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

func hashCommand() {
	// Create a new flag set for the hash subcommand
	hashFlags := flag.NewFlagSet("hash", flag.ExitOnError)
	dirPath := hashFlags.String("dir", "", "Directory path to hash (required)")
	help := hashFlags.Bool("help", false, "Show help message")

	hashFlags.Parse(os.Args[2:])

	// Show help if requested or no directory specified
	if *help || *dirPath == "" {
		showHashHelp()
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
	hfhRequest, err := hfh_cli.HFHrequestFromPath(absPath)
	if err != nil {
		log.Fatalf("Error calculating hashes: %v", err)
	}

	if hfhRequest == nil {
		log.Fatal("Error: No hash result returned (directory may be empty or all files filtered)")
	}

	// Parse the hash strings to uint64
	dirHash, err := strconv.ParseUint(hfhRequest.SimHashDirNames, 16, 64)
	if err != nil {
		log.Fatalf("Error parsing directory hash: %v", err)
	}

	nameHash, err := strconv.ParseUint(hfhRequest.SimHashNames, 16, 64)
	if err != nil {
		log.Fatalf("Error parsing names hash: %v", err)
	}

	contentHash, err := strconv.ParseUint(hfhRequest.SimHashContent, 16, 64)
	if err != nil {
		log.Fatalf("Error parsing content hash: %v", err)
	}

	// Create combined hash
	combinedHash := hfh.CreateCombinedHash(dirHash, nameHash, contentHash)

	// Output results
	fmt.Printf("\nHash Results:\n")
	fmt.Printf("-------------\n")
	fmt.Printf("Directory Hash:   %016x\n", dirHash)
	fmt.Printf("Names Hash:       %016x\n", nameHash)
	fmt.Printf("Contents Hash:    %016x\n", contentHash)
	fmt.Printf("Combined Hash:    %016x\n", combinedHash)

	fmt.Printf("\nHash Results (formatted for CSV):\n")
	fmt.Printf("----------------------------------\n")
	fmt.Printf("%016x,%016x,%016x,%016x\n",
		dirHash,
		nameHash,
		contentHash,
		combinedHash)
}

func searchCommand() {
	// Create a new flag set for the search subcommand
	searchFlags := flag.NewFlagSet("search", flag.ExitOnError)
	dirPath := searchFlags.String("dir", "", "Directory path to hash and search for similar projects (required)")
	host := searchFlags.String("host", "localhost", "Qdrant server host")
	port := searchFlags.Int("port", 6334, "Qdrant server port")
	collection := searchFlags.String("collection", "url_collection", "Qdrant collection name")
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
	hfhRequest, err := hfh_cli.HFHrequestFromPath(absPath)
	if err != nil {
		log.Fatalf("Error calculating hashes: %v", err)
	}

	if hfhRequest == nil {
		log.Fatal("Error: No hash result returned (directory may be empty or all files filtered)")
	}

	// Parse the hash strings to uint64
	dirHash, err := strconv.ParseUint(hfhRequest.SimHashDirNames, 16, 64)
	if err != nil {
		log.Fatalf("Error parsing directory hash: %v", err)
	}

	nameHash, err := strconv.ParseUint(hfhRequest.SimHashNames, 16, 64)
	if err != nil {
		log.Fatalf("Error parsing names hash: %v", err)
	}

	contentHash, err := strconv.ParseUint(hfhRequest.SimHashContent, 16, 64)
	if err != nil {
		log.Fatalf("Error parsing content hash: %v", err)
	}

	fmt.Printf("\nQuery Hashes:\n")
	fmt.Printf("  Directory Hash:   %016x\n", dirHash)
	fmt.Printf("  Names Hash:       %016x\n", nameHash)
	fmt.Printf("  Contents Hash:    %016x\n", contentHash)
	fmt.Printf("Searching for similar projects in Qdrant...\n")
	fmt.Printf("Host: %s:%d, Collection: %s, Top: %d\n", *host, *port, *collection, *topK)
	fmt.Println(repeatString("-", 60))

	// Search for similar projects in Qdrant
	config := hfh.QdrantConfig{
		Host:           *host,
		Port:           *port,
		CollectionName: *collection,
	}

	results, err := hfh.SearchSimilarProjects(config, dirHash, nameHash, contentHash, uint64(*topK))
	if err != nil {
		log.Fatalf("Error searching in Qdrant: %v", err)
	}

	if len(results) == 0 {
		fmt.Println("No similar projects found.")
		return
	}

	// Display results with enhanced information
	fmt.Printf("\nFound %d similar projects:\n", len(results))
	fmt.Println(repeatString("=", 80))

	for i, result := range results {
		fmt.Printf("\n%d. %s (Hamming Distance: %d)\n", i+1, result.SearchStage, result.HammingDist)

		// Display similarity score (if available)
		if result.Score > 0 {
			fmt.Printf("   Similarity Score: %.4f\n", result.Score)
		}

		fmt.Printf("   Vendor: %s\n", result.Vendor)
		fmt.Printf("   Component: %s\n", result.Component)
		fmt.Printf("   Version: %s\n", result.Version)

		if result.URL != "" {
			fmt.Printf("   URL: %s\n", result.URL)
		}

		// Show some additional metadata if available
		if license, ok := result.Metadata["license"]; ok && license != "" {
			fmt.Printf("   License: %v\n", license)
		}
		if totalFiles, ok := result.Metadata["total_files"]; ok {
			fmt.Printf("   Total Files: %v\n", totalFiles)
		}
		if size, ok := result.Metadata["size"]; ok {
			fmt.Printf("   Size: %v\n", size)
		}

		// Display hash information for debugging
		if dirHash, ok := result.Metadata["hfh_dirs_hash"]; ok {
			fmt.Printf("   Dir Hash: %v\n", dirHash)
		}
		if nameHash, ok := result.Metadata["hfh_names_hash"]; ok {
			fmt.Printf("   Names Hash: %v\n", nameHash)
		}
		if contentHash, ok := result.Metadata["hfh_contents_hash"]; ok {
			fmt.Printf("   Content Hash: %v\n", contentHash)
		}

		if i < len(results)-1 {
			fmt.Println(repeatString("-", 40))
		}
	}
}

func showHelp() {
	fmt.Println("HFH CLI - Hierarchical Folder Hashing Tool with Qdrant Search")
	fmt.Println("=============================================================")
	fmt.Println()
	fmt.Println("This tool can calculate folder hashes and search for similar projects in Qdrant.")
	fmt.Println("The search uses a multi-stage approach:")
	fmt.Println("  1. Exact hash matching - finds identical projects")
	fmt.Println("  2. Component-aware similarity - finds similar components (strict threshold)")
	fmt.Println("  3. General similarity - finds loosely similar projects (relaxed threshold)")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Println("  hash     Calculate hashes for a directory")
	fmt.Println("  search   Calculate hashes and search for similar projects in Qdrant")
	fmt.Println("  help     Show this help message")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hfh-cli <subcommand> [options]")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hfh-cli hash -dir /path/to/project")
	fmt.Println("  hfh-cli search -dir /path/to/project -top 5")
	fmt.Println("  hfh-cli search -dir . -host localhost -port 6334 -collection url_collection")
	fmt.Println()
	fmt.Println("For subcommand-specific help:")
	fmt.Println("  hfh-cli hash -help")
	fmt.Println("  hfh-cli search -help")
}

func showHashHelp() {
	fmt.Println("HFH CLI - Hash Subcommand")
	fmt.Println("=========================")
	fmt.Println()
	fmt.Println("Calculate three types of hashes for a directory:")
	fmt.Println("  1. Directory structure hash - based on folder hierarchy")
	fmt.Println("  2. File names hash - based on all file names")
	fmt.Println("  3. File contents hash - based on actual file contents")
	fmt.Println("  4. Combined hash - intelligent combination of all three")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hfh-cli hash -dir <directory_path>")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -dir string    Directory path to hash (required)")
	fmt.Println("  -help         Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hfh-cli hash -dir /path/to/project")
	fmt.Println("  hfh-cli hash -dir .")
	fmt.Println("  hfh-cli hash -dir ../some-folder")
	fmt.Println()
	fmt.Println("Note: Hidden files, build directories, and binary files are automatically filtered out.")
}

func showSearchHelp() {
	fmt.Println("HFH CLI - Search Subcommand")
	fmt.Println("===========================")
	fmt.Println()
	fmt.Println("Calculate hashes for a directory and search for similar projects in Qdrant.")
	fmt.Println("Uses a multi-stage approach with Hamming distance filtering:")
	fmt.Println()
	fmt.Println("Stage 1: Exact hash matching")
	fmt.Println("  - Searches for projects with identical hashes")
	fmt.Println("  - Returns immediately if exact matches found")
	fmt.Println()
	fmt.Println("Stage 2: Component-aware similarity (≤15 bit differences)")
	fmt.Println("  - Uses Manhattan distance for better Hamming approximation")
	fmt.Println("  - Applies strict threshold for high-quality matches")
	fmt.Println()
	fmt.Println("Stage 3: General similarity (≤30 bit differences)")
	fmt.Println("  - Fallback search with relaxed threshold")
	fmt.Println("  - Finds loosely similar projects")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hfh-cli search -dir <directory_path> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -dir string         Directory path to hash and search (required)")
	fmt.Println("  -host string        Qdrant server host (default: localhost)")
	fmt.Println("  -port int           Qdrant server port (default: 6334)")
	fmt.Println("  -collection string  Qdrant collection name (default: url_collection)")
	fmt.Println("  -top int            Number of top similar results to return (default: 10)")
	fmt.Println("  -help              Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hfh-cli search -dir /path/to/project")
	fmt.Println("  hfh-cli search -dir . -top 5")
	fmt.Println("  hfh-cli search -dir ../project -host 192.168.1.100 -port 6334")
	fmt.Println("  hfh-cli search -dir . -collection my_collection -top 20")
	fmt.Println()
	fmt.Println("Note: Requires a running Qdrant server with the specified collection.")
}

func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
