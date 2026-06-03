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

// Package main provides a tool to import CSV data into Qdrant collections.
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	progresstracker "github.com/scanoss/folder-hashing-api/internal/utils"
)

type WorkerConfig struct {
	NumWorkers           int
	RecommendedBatchSize int
}

const (
	// QdrantHost is the default Qdrant server hostname.
	QdrantHost = "localhost"
	// QdrantPort is the default Qdrant server port.
	QdrantPort = 6334
	// VectorDim is the dimensionality of the hash vectors.
	VectorDim = 64 // Single 64-bit hash per collection
	// DefaultWorkers is the default number of workers to use.
	DefaultWorkers = 2
)

var rankMap map[string]int

//nolint:gocyclo // Main function complexity is acceptable for CLI setup
func main() {
	csvDir := flag.String("dir", "", "Directory containing CSV files (required)")
	overwrite := flag.Bool("overwrite", false, "If true, delete existing collections before import")
	topPurlsPath := flag.String("top-purls", "", "File with top rated purls (required)")
	qdrantHost := flag.String("qdrant-host", QdrantHost, "Qdrant server host")
	qdrantPort := flag.Int("qdrant-port", QdrantPort, "Qdrant server port")

	flag.Parse()

	if *csvDir == "" {
		log.Fatal("Error: You must specify a directory with the -dir option")
	}

	if *topPurlsPath == "" {
		log.Fatal("Error: You must specify a file with the -top-purls option")
	}

	// Verify that the directory exists
	if _, err := os.Stat(*csvDir); os.IsNotExist(err) {
		log.Fatalf("Error: Directory %s does not exist", *csvDir)
	}

	var err error
	rankMap, err = initPurlMap(*topPurlsPath)
	if err != nil {
		log.Println("Warning: ", err)
	}

	// Start the timer to measure performance
	startTime := time.Now()

	// Connect to Qdrant
	log.Println("Connecting to Qdrant server...")
	ctx := context.Background()

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: *qdrantHost,
		Port: *qdrantPort,
	})
	if err != nil {
		log.Fatalf("Error connecting to Qdrant: %v", err)
	}
	defer func() {
		log.Println("Closing connection to Qdrant")
		if err := client.Close(); err != nil {
			log.Printf("Warning: Error closing Qdrant connection: %v", err)
		}
	}()
	log.Println("Connected to Qdrant server successfully")

	collections := entities.GetAllSupportedCollections()

	log.Printf("Will create/update %d collections: %v", len(collections), collections)

	// Check and create collections
	for _, collectionName := range collections {
		log.Printf("Checking collection: %s", collectionName)
		collectionExists, err := client.CollectionExists(ctx, collectionName)
		if err != nil {
			log.Printf("Error checking if collection %s exists: %v", collectionName, err)
			return
		}

		if *overwrite && collectionExists {
			log.Printf("Collection %s exists and overwrite flag is set. Dropping collection...", collectionName)
			err = client.DeleteCollection(ctx, collectionName)
			if err != nil {
				//nolint:gocritic // Error is fatal, defer will not help here
				log.Fatalf("Error dropping collection %s: %v", collectionName, err)
			}
			log.Printf("Collection '%s' dropped successfully", collectionName)
			collectionExists = false
		}

		// Create collection if it doesn't exist
		if !collectionExists {
			createCollection(ctx, client, collectionName)
		} else {
			log.Printf("Using existing collection: %s", collectionName)
		}
	}

	// Get list of CSV files in the directory
	log.Printf("Reading directory '%s' for CSV files...", *csvDir)
	files, err := os.ReadDir(*csvDir)
	if err != nil {
		log.Fatalf("Error reading directory: %v", err)
	}

	var csvFiles []string
	var csvFilesSize int64
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".csv") {
			csvFiles = append(csvFiles, filepath.Join(*csvDir, file.Name()))

			fileInfo, err := os.Stat(filepath.Join(*csvDir, file.Name()))
			if err != nil {
				log.Printf("Error getting file size for %s: %v", file.Name(), err)
				continue
			}
			csvFilesSize += fileInfo.Size()
		}
	}

	log.Printf("Found %d CSV files to import", len(csvFiles))
	if len(csvFiles) == 0 {
		log.Println("No CSV files found. Exiting.")
		return
	}

	progress := progresstracker.NewProgressTracker(len(csvFiles))

	// Channel to process files
	filesChan := make(chan string, len(csvFiles))
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(csvFiles))

	avgFileSizeBytes := csvFilesSize / int64(len(csvFiles))
	avgFileSizeMB := int(avgFileSizeBytes / (1024 * 1024))
	if avgFileSizeMB == 0 {
		avgFileSizeMB = 1
	}
	optimalWorkers := CalculateOptimalWorkers(len(csvFiles), avgFileSizeMB)

	// Start workers to process files in parallel
	log.Printf("Starting %d worker(s) to process CSV files...", optimalWorkers.NumWorkers)
	for workerID := range optimalWorkers.NumWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for file := range filesChan {
				recordCount, err := importCSVFileWithProgress(ctx, client, file, optimalWorkers.RecommendedBatchSize, progress)
				if err != nil {
					log.Printf("Worker %d: Error importing file %s: %v", workerID, file, err)
					errorsChan <- fmt.Errorf("error importing file %s: %w", file, err)
					progress.FileCompleted(0, false)
				} else {
					progress.FileCompleted(recordCount, true)
				}
			}
		}(workerID)
	}

	// Send files to workers
	for _, file := range csvFiles {
		filesChan <- file
	}
	close(filesChan)

	// Wait for all workers to finish
	log.Println("Waiting for all workers to complete...")
	wg.Wait()
	close(errorsChan)

	// Wait for all progress bars to complete rendering
	progress.Wait()

	progress.PrintFinalSummary()

	// Check if there were any errors during processing
	errCount := 0
	for err := range errorsChan {
		errCount++
		log.Printf("Import error: %v", err)
	}
	if errCount > 0 {
		log.Printf("WARNING: Encountered %d errors during import", errCount)
	}

	// Show statistics
	elapsed := time.Since(startTime)
	log.Printf("Import process completed. Total time: %s", elapsed)

	// Show collection statistics if possible
	for _, collectionName := range collections {
		showCollectionStats(ctx, client, collectionName)
	}

	// Re-enable production HNSW indexing for all collections
	log.Println("\n=== Enabling Production HNSW Indexing ===")
	log.Println("Re-enabling HNSW indexing (M=48) for production queries...")
	for _, collectionName := range collections {
		if err := enableProductionIndexing(ctx, client, collectionName); err != nil {
			log.Printf("WARNING: Failed to enable production indexing for %s: %v", collectionName, err)
		}
	}
	log.Println("\n✓ Production indexing enabled for all collections.")
	log.Println("The Qdrant optimizer will build HNSW indexes in the background.")
	log.Println("Monitor collection stats to track indexing progress.")
}

// Create a language-based collection with named vectors (dirs, names, contents).
func createCollection(ctx context.Context, client *qdrant.Client, collectionName string) {
	log.Printf("Creating language-based collection with named vectors: %s", collectionName)

	// Create named vectors configuration for dirs, names, and contents
	// Optimized for bulk import: vectors on disk, HNSW disabled (M=0)
	namedVectors := map[string]*qdrant.VectorParams{
		"dirs": {
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			OnDisk:   qdrant.PtrOf(true), // Store vectors on disk to reduce RAM during import
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(0)), // Disable HNSW during import, re-enabled after
				EfConstruct:       qdrant.PtrOf(uint64(500)),
				FullScanThreshold: qdrant.PtrOf(uint64(100000)),
				OnDisk:            qdrant.PtrOf(true), // Store HNSW index on disk
			},
		},
		"names": {
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			OnDisk:   qdrant.PtrOf(true), // Store vectors on disk to reduce RAM during import
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(0)), // Disable HNSW during import, re-enabled after
				EfConstruct:       qdrant.PtrOf(uint64(500)),
				FullScanThreshold: qdrant.PtrOf(uint64(100000)),
				OnDisk:            qdrant.PtrOf(true), // Store HNSW index on disk
			},
		},
		"contents": {
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			OnDisk:   qdrant.PtrOf(true), // Store vectors on disk to reduce RAM during import
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(0)), // Disable HNSW during import, re-enabled after
				EfConstruct:       qdrant.PtrOf(uint64(500)),
				FullScanThreshold: qdrant.PtrOf(uint64(100000)),
				OnDisk:            qdrant.PtrOf(true), // Store HNSW index on disk
			},
		},
	}

	// Create collection with named vectors
	err := client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig:  qdrant.NewVectorsConfigMap(namedVectors),
		// Aggressive optimization for large collections
		OptimizersConfig: &qdrant.OptimizersConfigDiff{
			DefaultSegmentNumber: qdrant.PtrOf(uint64(32)),     // Many segments for parallelism
			MaxSegmentSize:       qdrant.PtrOf(uint64(500000)), // Large segments for efficiency
		},
		// Binary quantization for memory efficiency
		QuantizationConfig: &qdrant.QuantizationConfig{
			Quantization: &qdrant.QuantizationConfig_Binary{
				Binary: &qdrant.BinaryQuantization{
					AlwaysRam: qdrant.PtrOf(false), // Allow quantized vectors on disk to reduce RAM
				},
			},
		},
	})
	if err != nil {
		log.Fatalf("Error creating collection %s: %v", collectionName, err)
	}
	log.Printf("Collection '%s' with named vectors created successfully", collectionName)

	// Create indexes for faster filtering
	log.Printf("Creating payload indexes for collection %s...", collectionName)

	// Index for component fields used in grouping and filtering
	textFields := []string{"purl", "version"}
	for _, field := range textFields {
		_, err = client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: collectionName,
			FieldName:      field,
			FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeKeyword),
		})
		if err != nil {
			log.Printf("Warning: Could not create index for %s in %s: %v", field, collectionName, err)
		} else {
			log.Printf("Created index for field: %s in %s", field, collectionName)
		}
	}

	// Create rank index
	_, err = client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: collectionName,
		FieldName:      "rank",
		FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeInteger),
	})
	if err != nil {
		log.Printf("Warning: Could not create index for rank in %s: %v", collectionName, err)
	} else {
		log.Printf("Created index for field: rank in %s", collectionName)
	}
}

// enableProductionIndexing re-enables HNSW indexing after bulk import is complete.
// This should be called after all data has been imported to optimize for production queries.
func enableProductionIndexing(ctx context.Context, client *qdrant.Client, collectionName string) error {
	log.Printf("Enabling production HNSW indexing for collection: %s", collectionName)

	// Build named vectors config map
	namedVectorsConfig := make(map[string]*qdrant.VectorParamsDiff)
	for _, vectorName := range []string{"dirs", "names", "contents"} {
		namedVectorsConfig[vectorName] = &qdrant.VectorParamsDiff{
			HnswConfig: &qdrant.HnswConfigDiff{
				M:      qdrant.PtrOf(uint64(48)),
				OnDisk: qdrant.PtrOf(false),
			},
			OnDisk: qdrant.PtrOf(true), // Keep vectors on disk, only HNSW in RAM
		}
	}

	// Update all named vectors and collection settings in a single call
	err := client.UpdateCollection(ctx, &qdrant.UpdateCollection{
		CollectionName: collectionName,
		VectorsConfig: &qdrant.VectorsConfigDiff{
			Config: &qdrant.VectorsConfigDiff_ParamsMap{
				ParamsMap: &qdrant.VectorParamsDiffMap{
					Map: namedVectorsConfig,
				},
			},
		},
		OptimizersConfig: &qdrant.OptimizersConfigDiff{
			IndexingThreshold: qdrant.PtrOf(uint64(0)),
		},
		QuantizationConfig: &qdrant.QuantizationConfigDiff{
			Quantization: &qdrant.QuantizationConfigDiff_Binary{
				Binary: &qdrant.BinaryQuantization{
					AlwaysRam: qdrant.PtrOf(true),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update HNSW config: %w", err)
	}

	log.Printf("✓ HNSW indexing enabled for %s. Optimizer will build indexes in background.", collectionName)
	return nil
}

// Import data from a CSV file to separate collections.
func importCSVFileWithProgress(ctx context.Context, client *qdrant.Client, filePath string, batchSize int, progress *progresstracker.ProgressTracker) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("error opening CSV file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Warning: Error closing file %s: %v", filePath, err)
		}
	}()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = 0
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	batch := make([][]string, 0, batchSize)
	totalRecords := 0
	batchNum := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			if len(batch) > 0 {
				collectionCounts, err := insertBatchToCollections(ctx, client, batch)
				if err != nil {
					log.Printf("WARNING: Error inserting final batch: %v", err)
				} else {
					// Update progress for each collection
					for collection, count := range collectionCounts {
						progress.AddRecords(count, collection)
					}
					totalRecords += len(batch)
				}
			}
			break
		}

		if err != nil {
			log.Printf("WARNING: Error reading line in %s: %v", filePath, err)
			continue
		}

		batch = append(batch, record)

		if len(batch) >= batchSize {
			batchNum++
			// Progress bars will show batch processing status

			collectionCounts, err := insertBatchToCollections(ctx, client, batch)
			if err != nil {
				log.Printf("WARNING: Error inserting batch %d: %v", batchNum, err)
			} else {
				// Update progress for each collection
				for collection, count := range collectionCounts {
					progress.AddRecords(count, collection)
				}
				totalRecords += len(batch)
			}

			batch = make([][]string, 0, batchSize)
		}
	}

	// File completed successfully - progress bar will reflect this
	return totalRecords, nil
}

// hexSimhashToVector converts hex hash string to vector.
func hexSimhashToVector(hexHashString string, bits int) ([]float32, error) {
	if hexHashString == "" {
		return nil, fmt.Errorf("input hex string cannot be empty")
	}

	uintValue, err := strconv.ParseUint(hexHashString, 16, bits)
	if err != nil {
		return nil, fmt.Errorf("could not parse hex string '%s': %w", hexHashString, err)
	}

	formatString := fmt.Sprintf("%%0%db", bits)
	binaryString := fmt.Sprintf(formatString, uintValue)

	if len(binaryString) != bits {
		return nil, fmt.Errorf("internal error: formatted binary string length (%d) does not match expected bits (%d)", len(binaryString), bits)
	}

	vector := make([]float32, bits)
	for i, bitRune := range binaryString {
		if bitRune == '1' {
			vector[i] = 1.0
		} else {
			vector[i] = 0.0
		}
	}

	return vector, nil
}

// Insert a batch of records to language-based collections.
//
//nolint:gocyclo,nestif // Batch processing complexity is inherent to CSV import
func insertBatchToCollections(ctx context.Context, client *qdrant.Client, batch [][]string) (map[string]int, error) {
	// Group points by collection (language)
	collectionPoints := make(map[string][]*qdrant.PointStruct)

	// Process each record in the batch
	for _, record := range batch {
		if len(record) < 13 {
			log.Printf("WARNING: Skipping incomplete record with %d fields", len(record))
			continue
		}

		// Parse hash values
		hfhDirsStr := strings.TrimSpace(record[0])
		hfhNamesStr := strings.TrimSpace(record[1])
		hfhContentsStr := strings.TrimSpace(record[2])
		urlHashStr := strings.TrimSpace(record[3])
		urlMd5Str := strings.TrimSpace(record[4])

		// Skip if any critical hash is invalid
		if hfhDirsStr == "" || hfhNamesStr == "" || hfhContentsStr == "" {
			continue
		}

		dirVector, err := hexSimhashToVector(hfhDirsStr, VectorDim)
		if err != nil {
			continue
		}
		nameVector, err := hexSimhashToVector(hfhNamesStr, VectorDim)
		if err != nil {
			continue
		}
		contentVector, err := hexSimhashToVector(hfhContentsStr, VectorDim)
		if err != nil {
			continue
		}

		purl := strings.TrimSpace(record[5])
		vendor := strings.TrimSpace(record[6])
		component := strings.TrimSpace(record[7])
		version := strings.TrimSpace(record[8])
		releaseDate := strings.TrimSpace(record[9])
		license := strings.TrimSpace(record[10])

		// Parse rank from the last CSV column. Defaults to 0 on failure.
		rank, err := strconv.Atoi(strings.TrimSpace(record[12]))
		if err != nil {
			rank = 0
		}
		// If the PURL is on the preferred list of purls, override the CSV value
		if r, exists := rankMap[purl]; exists {
			rank = r
		}

		// Generate unique point ID to handle re-imports idempotently
		idStringToHash := fmt.Sprintf("%s|%s|%s|%s|%s|%s", purl, version, hfhDirsStr, hfhNamesStr, hfhContentsStr, urlHashStr)
		hasher := fnv.New64a()
		hasher.Write([]byte(idStringToHash))
		pointID := hasher.Sum64()

		// Common payload for all collections
		payload := map[string]*qdrant.Value{
			"vendor":        qdrant.NewValueString(vendor),
			"component":     qdrant.NewValueString(component),
			"version":       qdrant.NewValueString(version),
			"release_date":  qdrant.NewValueString(releaseDate),
			"license":       qdrant.NewValueString(license),
			"purl":          qdrant.NewValueString(purl),
			"url_hash":      qdrant.NewValueString(urlHashStr),
			"url_md5":       qdrant.NewValueString(urlMd5Str),
			"dirs_hash":     qdrant.NewValueString(hfhDirsStr),
			"names_hash":    qdrant.NewValueString(hfhNamesStr),
			"contents_hash": qdrant.NewValueString(hfhContentsStr),
			"rank":          qdrant.NewValueInt(int64(rank)),
		}

		// Parse language extensions (column 11) to determine target collection
		targetCollection := "misc_collection" // default fallback
		var langExtensions map[string]int

		if strings.TrimSpace(record[11]) != "" {
			langExtStr := strings.TrimSpace(record[11])

			if err := json.Unmarshal([]byte(langExtStr), &langExtensions); err != nil {
				// Failed to parse JSON - use misc_collection
				payload["language_extensions"] = qdrant.NewValueString(langExtStr)
			} else {
				// Convert to Qdrant struct format for proper indexing
				langExtStruct := make(map[string]*qdrant.Value)
				for lang, count := range langExtensions {
					langExtStruct[lang] = qdrant.NewValueInt(int64(count))
				}
				payload["language_extensions"] = qdrant.NewValueStruct(&qdrant.Struct{
					Fields: langExtStruct,
				})

				// Determine target collection based on language extensions
				langExt := make(entities.LanguageExtensions)
				for lang, count := range langExtensions {
					// Safe conversion with overflow check
					if count > 2147483647 {
						langExt[lang] = 2147483647
					} else {
						langExt[lang] = int32(count) // #nosec G115 -- checked above
					}
				}
				targetCollection = entities.GetCollectionNameFromLanguageExtensions(langExt)
			}
		}

		// Create point with named vectors for the target collection
		vectorsMap := map[string]*qdrant.Vector{
			"dirs":     qdrant.NewVector(dirVector...),
			"names":    qdrant.NewVector(nameVector...),
			"contents": qdrant.NewVector(contentVector...),
		}

		point := &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointID),
			Vectors: qdrant.NewVectorsMap(vectorsMap),
			Payload: payload,
		}

		// Add to the appropriate collection bucket
		collectionPoints[targetCollection] = append(collectionPoints[targetCollection], point)
	}

	if len(collectionPoints) == 0 {
		return nil, nil
	}

	// Insert to collections in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, len(collectionPoints))

	// Track counts per collection
	collectionCounts := make(map[string]int)

	for collectionName, points := range collectionPoints {
		if len(points) > 0 {
			wg.Add(1)
			go func(colName string, pts []*qdrant.PointStruct) {
				defer wg.Done()
				_, err := client.Upsert(ctx, &qdrant.UpsertPoints{
					CollectionName: colName,
					Points:         pts,
				})
				if err != nil {
					errChan <- fmt.Errorf("error inserting to %s collection: %w", colName, err)
				} else {
					mu.Lock()
					collectionCounts[colName] = len(pts)
					mu.Unlock()
				}
			}(collectionName, points)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return collectionCounts, err
		}
	}

	return collectionCounts, nil
}

// Function to show collection statistics.
func showCollectionStats(ctx context.Context, client *qdrant.Client, collectionName string) {
	log.Printf("\n=== Collection Statistics (%s) ===", collectionName)

	info, err := client.GetCollectionInfo(ctx, collectionName)
	if err != nil {
		log.Printf("Could not retrieve collection information for %s: %v", collectionName, err)
		return
	}

	log.Printf("  Status: %s", info.Status)
	log.Printf("  Points count: %d", info.PointsCount)
	log.Printf("  Segments count: %d", info.SegmentsCount)
}

func initPurlMap(filename string) (map[string]int, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Warning: Error closing file %s: %v", filename, err)
		}
	}()

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var result map[string]int

	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetAvailableMemoryMB returns available memory in MB.
func GetAvailableMemoryMB() (int, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Warning: Error closing file: %v", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err != nil {
					return 0, err
				}
				return kb / 1024, nil // Convert KB to MB
			}
		}
	}
	return 0, scanner.Err()
}

// CalculateOptimalWorkers calculates the optimal number of workers for processing CSV files,
// based on the number of files and their average size.
// It takes into account available memory and CPU cores to determine the optimal number of workers.
func CalculateOptimalWorkers(fileCount, avgFileSizeMB int) WorkerConfig {
	numCPU := runtime.NumCPU()

	log.Printf("Number of CPU cores: %d", numCPU)

	// Get available memory
	availableMemMB, err := GetAvailableMemoryMB()
	if err != nil {
		log.Printf("Error getting memory info: %v, falling back to conservative estimate", err)
		availableMemMB = 1024 // 1GB fallback
	}

	log.Printf("Available memory: %d MB", availableMemMB)

	// Estimate memory per worker (rough calculation)
	// Each worker might hold BatchSize * record size in memory
	estimatedMemPerWorkerMB := avgFileSizeMB + 100
	// Calculate max workers based on memory
	maxWorkersByMem := availableMemMB / estimatedMemPerWorkerMB / 2 // Use only 50% of available

	// Calculate based on CPU (I/O bound: 2x cores)
	maxWorkersByCPU := numCPU * 2

	// Take the minimum of constraints
	optimalWorkers := int(math.Min(
		float64(maxWorkersByMem),
		float64(maxWorkersByCPU),
	))

	// Cap at file count and set bounds between 2 and 32
	if optimalWorkers < 2 {
		optimalWorkers = 2
	}
	if optimalWorkers > fileCount {
		optimalWorkers = fileCount
	}
	if optimalWorkers > 32 {
		optimalWorkers = 32
	}

	log.Printf("Optimal numbers of workers: %d", optimalWorkers)

	// Adjust batch size based on workers
	recommendedBatchSize := 2000
	if optimalWorkers > 16 {
		// More workers = smaller batches for better distribution
		recommendedBatchSize = 1000
	}

	return WorkerConfig{
		NumWorkers:           optimalWorkers,
		RecommendedBatchSize: recommendedBatchSize,
	}
}
