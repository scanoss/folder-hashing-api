// Package main provides a tool to import CSV data into Qdrant collections.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

const (
	// QdrantHost is the default Qdrant server hostname.
	QdrantHost = "localhost"
	// QdrantPort is the default Qdrant server port.
	QdrantPort = 6334
	// BatchSize is the number of records to process in each batch.
	BatchSize = 2000 // Larger batch size for better performance
	// MaxWorkers is the number of parallel workers for file processing.
	MaxWorkers = 12 // More workers for parallel processing
	// VectorDim is the dimensionality of the hash vectors.
	VectorDim = 64 // Single 64-bit hash per collection
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

	log.Printf("Will create/check %d language-based collections: %v", len(collections), collections)

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
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".csv") {
			csvFiles = append(csvFiles, filepath.Join(*csvDir, file.Name()))
		}
	}

	log.Printf("Found %d CSV files to import", len(csvFiles))
	if len(csvFiles) == 0 {
		log.Println("No CSV files found. Exiting.")
		return
	}

	// Channel to process files
	filesChan := make(chan string, len(csvFiles))
	var wg sync.WaitGroup

	// Error channel to collect errors from workers
	errorsChan := make(chan error, len(csvFiles))

	// Start workers to process files in parallel
	log.Printf("Starting %d worker(s) to process CSV files...", MaxWorkers)
	for i := 0; i < MaxWorkers; i++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			for file := range filesChan {
				sectorName := filepath.Base(file)
				sectorName = strings.TrimSuffix(sectorName, ".csv")
				log.Printf("Worker %d: Processing sector %s", workerId, sectorName)

				err := importCSVFile(ctx, client, file, sectorName)
				if err != nil {
					log.Printf("Worker %d: Error importing file %s: %v", workerId, file, err)
					errorsChan <- fmt.Errorf("error importing file %s: %w", file, err)
				} else {
					log.Printf("Worker %d: Successfully processed sector %s", workerId, sectorName)
				}
			}
		}(i)
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
}

// Create a language-based collection with named vectors (dirs, names, contents).
func createCollection(ctx context.Context, client *qdrant.Client, collectionName string) {
	log.Printf("Creating language-based collection with named vectors: %s", collectionName)

	// Create named vectors configuration for dirs, names, and contents
	namedVectors := map[string]*qdrant.VectorParams{
		"dirs": {
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(48)),
				EfConstruct:       qdrant.PtrOf(uint64(500)),
				FullScanThreshold: qdrant.PtrOf(uint64(100000)),
			},
		},
		"names": {
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(48)),
				EfConstruct:       qdrant.PtrOf(uint64(500)),
				FullScanThreshold: qdrant.PtrOf(uint64(100000)),
			},
		},
		"contents": {
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(48)),
				EfConstruct:       qdrant.PtrOf(uint64(500)),
				FullScanThreshold: qdrant.PtrOf(uint64(100000)),
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
			IndexingThreshold:    qdrant.PtrOf(uint64(100000)), // High threshold for performance
		},
		// Binary quantization for memory efficiency
		QuantizationConfig: &qdrant.QuantizationConfig{
			Quantization: &qdrant.QuantizationConfig_Binary{
				Binary: &qdrant.BinaryQuantization{
					AlwaysRam: qdrant.PtrOf(true), // Keep quantized vectors in RAM
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

	// Index for component fields and category for faster grouping and filtering
	textFields := []string{"purl", "version", "url", "category"}
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

// Import data from a CSV file to separate collections.
func importCSVFile(ctx context.Context, client *qdrant.Client, filePath, sectorName string) error {
	log.Printf("Opening CSV file: %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Warning: Error closing file %s: %v", filePath, err)
		}
	}()

	validRecords := make([][]string, 0, 10000) // Pre-allocate for performance
	var lineNumber int
	var errorCount int

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = 0
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	for {
		lineNumber++
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("WARNING: Error reading line %d in file %s: %v", lineNumber, filePath, err)
			continue
		}
		validRecords = append(validRecords, record)
	}

	totalRecords := len(validRecords)
	if totalRecords == 0 {
		log.Printf("No valid records found in %s, skipping", filePath)
		return nil
	}

	if errorCount > 0 {
		log.Printf("Processed %s: %d valid records, %d parsing errors", filePath, totalRecords, errorCount)
	}

	log.Printf("Importing %d valid records from sector %s to separate collections", totalRecords, sectorName)

	// Process in batches for better performance
	batchesProcessed := 0
	for i := 0; i < totalRecords; i += BatchSize {
		end := i + BatchSize
		if end > totalRecords {
			end = totalRecords
		}
		batch := validRecords[i:end]
		batchNum := i/BatchSize + 1
		log.Printf("Processing batch %d/%d (%d records) for sector %s",
			batchNum, (totalRecords+BatchSize-1)/BatchSize, len(batch), sectorName)

		// Insert to all three collections in parallel
		err := insertBatchToSeparateCollections(ctx, client, batch)
		if err != nil {
			log.Printf("WARNING: Error inserting batch %d: %v. Continuing with next batch.", batchNum, err)
			continue
		}

		batchesProcessed++
	}

	log.Printf("All %d batches for sector %s imported successfully", batchesProcessed, sectorName)
	return nil
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
func insertBatchToSeparateCollections(ctx context.Context, client *qdrant.Client, batch [][]string) error {
	// Group points by collection (language)
	collectionPoints := make(map[string][]*qdrant.PointStruct)

	// Process each record in the batch
	for _, record := range batch {
		if len(record) < 17 {
			log.Printf("WARNING: Skipping incomplete record with %d fields", len(record))
			continue
		}

		// Parse hash values
		hfhDirsStr := strings.TrimSpace(record[0])
		hfhNamesStr := strings.TrimSpace(record[1])
		hfhContentsStr := strings.TrimSpace(record[2])
		urlHashStr := strings.TrimSpace(record[3])

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

		// Parse metadata (default to 0 if parsing fails)
		totalFiles, err := strconv.ParseInt(record[11], 10, 32)
		if err != nil {
			totalFiles = 0
		}
		indexedFiles, err := strconv.ParseInt(record[12], 10, 32)
		if err != nil {
			indexedFiles = 0
		}
		sourceFiles, err := strconv.ParseInt(record[13], 10, 32)
		if err != nil {
			sourceFiles = 0
		}
		ignoredFiles, err := strconv.ParseInt(record[14], 10, 32)
		if err != nil {
			ignoredFiles = 0
		}
		size, err := strconv.ParseInt(record[15], 10, 32)
		if err != nil {
			size = 0
		}
		categoryStr := strings.TrimSpace(record[16])

		// Generate unique point ID based on metadata to handle re-imports gracefully
		component := strings.TrimSpace(record[5])
		vendor := strings.TrimSpace(record[4])
		version := strings.TrimSpace(record[6])
		url := strings.TrimSpace(record[10])

		idStringToHash := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s", vendor, component, version, url, categoryStr, hfhDirsStr, hfhNamesStr, hfhContentsStr, urlHashStr)
		categoryToRank := map[string]int{"github_popular": 3, "github": 5, "common": 6, "forks": 9}
		rank := categoryToRank[categoryStr]
		hasher := fnv.New64a()
		hasher.Write([]byte(idStringToHash))
		pointID := hasher.Sum64()

		// If the PURL is on the preferred list of purls, take that value
		purl := strings.TrimSpace(record[9])
		if r, exists := rankMap[purl]; exists {
			rank = r
		}

		// Common payload for all collections
		payload := map[string]*qdrant.Value{
			"vendor":        qdrant.NewValueString(strings.TrimSpace(record[4])),
			"component":     qdrant.NewValueString(strings.TrimSpace(record[5])),
			"version":       qdrant.NewValueString(strings.TrimSpace(record[6])),
			"release_date":  qdrant.NewValueString(strings.TrimSpace(record[7])),
			"license":       qdrant.NewValueString(strings.TrimSpace(record[8])),
			"purl":          qdrant.NewValueString(strings.TrimSpace(record[9])),
			"url":           qdrant.NewValueString(strings.TrimSpace(record[10])),
			"url_hash":      qdrant.NewValueString(urlHashStr),
			"dirs_hash":     qdrant.NewValueString(hfhDirsStr),
			"names_hash":    qdrant.NewValueString(hfhNamesStr),
			"contents_hash": qdrant.NewValueString(hfhContentsStr),
			"total_files":   qdrant.NewValueInt(totalFiles),
			"indexed_files": qdrant.NewValueInt(indexedFiles),
			"source_files":  qdrant.NewValueInt(sourceFiles),
			"ignored_files": qdrant.NewValueInt(ignoredFiles),
			"size":          qdrant.NewValueInt(size),
			"category":      qdrant.NewValueString(categoryStr),
			"rank":          qdrant.NewValueInt(int64(rank)),
		}

		// Parse language extensions if present (column 17) to determine target collection
		targetCollection := "misc_collection" // default fallback
		var langExtensions map[string]int

		if len(record) > 17 && strings.TrimSpace(record[17]) != "" {
			langExtStr := strings.TrimSpace(record[17])

			if err := json.Unmarshal([]byte(langExtStr), &langExtensions); err != nil {
				log.Printf("WARNING: Failed to parse language_extensions JSON '%s': %v. Using misc_collection.", langExtStr, err)
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
		return nil
	}

	// Insert to language-based collections in parallel using goroutines
	var wg sync.WaitGroup
	errChan := make(chan error, len(collectionPoints))

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
					log.Printf("Successfully inserted %d points to %s", len(pts), colName)
				}
			}(collectionName, points)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
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
