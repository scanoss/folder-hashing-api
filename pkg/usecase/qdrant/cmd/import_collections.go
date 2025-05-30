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
	"scanoss.com/hfh-api/pkg/hfh"
)

const (
	QdrantHost = "localhost"
	QdrantPort = 6334
	BatchSize  = 2000 // Larger batch size for better performance
	MaxWorkers = 12   // More workers for parallel processing
	VectorDim  = 64   // Single 64-bit hash per collection
)

// Separate collection names for better performance
var (
	DirsCollectionName     = "dirs_collection"
	NamesCollectionName    = "names_collection"
	ContentsCollectionName = "contents_collection"
)

func main() {
	// Process command line arguments
	csvDir := flag.String("dir", "", "Directory containing CSV files (required)")
	overwriteFlag := flag.Bool("overwrite", false, "If true, delete existing collections before import")

	flag.Parse()

	if *csvDir == "" {
		log.Fatal("Error: You must specify a directory with the -dir option")
	}

	// Verify that the directory exists
	if _, err := os.Stat(*csvDir); os.IsNotExist(err) {
		log.Fatalf("Error: Directory %s does not exist", *csvDir)
	}

	// Start the timer to measure performance
	startTime := time.Now()

	// Connect to Qdrant
	log.Println("Connecting to Qdrant server...")
	ctx := context.Background()

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: QdrantHost,
		Port: QdrantPort,
	})
	if err != nil {
		log.Fatalf("Error connecting to Qdrant: %v", err)
	}
	defer func() {
		log.Println("Closing connection to Qdrant")
		client.Close()
	}()
	log.Println("Connected to Qdrant server successfully")

	// Collection names
	collections := []string{DirsCollectionName, NamesCollectionName, ContentsCollectionName}

	// Check and create collections
	for _, collectionName := range collections {
		log.Printf("Checking collection: %s", collectionName)
		collectionExists, err := client.CollectionExists(ctx, collectionName)
		if err != nil {
			log.Fatalf("Error checking if collection %s exists: %v", collectionName, err)
		}

		if *overwriteFlag && collectionExists {
			log.Printf("Collection %s exists and overwrite flag is set. Dropping collection...", collectionName)
			err = client.DeleteCollection(ctx, collectionName)
			if err != nil {
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
					errorsChan <- fmt.Errorf("error importing file %s: %v", file, err)
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

// Create a single collection optimized for large-scale approximate search
func createCollection(ctx context.Context, client *qdrant.Client, collectionName string) {
	log.Printf("Creating optimized collection: %s", collectionName)

	// Create collection with aggressive optimization for large collections
	err := client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan,
			HnswConfig: &qdrant.HnswConfigDiff{
				M:                 qdrant.PtrOf(uint64(48)),     // Higher M for better connectivity
				EfConstruct:       qdrant.PtrOf(uint64(500)),    // Higher construction for better index quality
				FullScanThreshold: qdrant.PtrOf(uint64(100000)), // Much higher threshold - no full scans
			},
		}),
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
	log.Printf("Collection '%s' created successfully", collectionName)

	// Create indexes for faster filtering
	log.Printf("Creating payload indexes for collection %s...", collectionName)

	// Index for component fields for faster grouping
	textFields := []string{"component", "vendor", "version", "url"}
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

	// Index for language extension fields for faster filtering. Move this to a const array
	langExtensions := []string{
		// Web/Frontend
		"ts", "js", "jsx", "tsx", "html", "css", "scss", "less", "vue", "svelte",
		// Backend/General
		"py", "java", "class", "jar", "go", "rb", "php", "cs", "rs", "scala", "kt", "groovy", "clj", "ex", "exs",
		// C-family
		"c", "h", "cpp", "cxx", "cc", "hpp", "hxx", "m", "mm", "swift",
		// Shell/Scripts
		"sh", "bash", "zsh", "ps1", "bat", "cmd", "pl", "pm", "t",
		// Data/Config
		"json", "yaml", "yml", "xml", "toml", "ini", "conf", "cfg", "properties",
		// Documentation
		"md", "rst", "txt", "tex", "adoc", "wiki",
		// Mobile
		"dart", "kotlin", "swift", "gradle",
		// Database
		"sql", "graphql", "prisma",
		// Other
		"lua", "r", "d", "fs", "f", "f90", "hs", "erl", "elm", "lisp", "jl",
		// Empty extension (for files without extension)
		"",
	}
	for _, field := range langExtensions {
		_, err = client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: collectionName,
			FieldName:      fmt.Sprintf("language_extensions.%s", field),
			FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeInteger),
		})
		if err != nil {
			log.Printf("Warning: Could not create index for %s in %s: %v", field, collectionName, err)
		} else {
			log.Printf("Created index for field: %s in %s", field, collectionName)
		}
	}
}

// Import data from a CSV file to separate collections
func importCSVFile(ctx context.Context, client *qdrant.Client, filePath, sectorName string) error {
	log.Printf("Opening CSV file: %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %v", err)
	}
	defer file.Close()

	var validRecords [][]string
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

// Insert a batch of records to separate collections in parallel
func insertBatchToSeparateCollections(ctx context.Context, client *qdrant.Client, batch [][]string) error {
	var dirsPoints, namesPoints, contentsPoints []*qdrant.PointStruct

	// Process each record in the batch
	for _, record := range batch {
		if len(record) < 17 {
			log.Printf("WARNING: Skipping incomplete record with %d fields", len(record))
			continue
		}

		// Parse hash values
		hfhDirsStr := strings.TrimSpace(record[0])
		hfhNamesStr := strings.TrimSpace(record[1])
		hfhContentsStr := strings.TrimSpace(record[3])
		urlHashStr := strings.TrimSpace(record[4])

		// Skip if any critical hash is invalid
		if hfhDirsStr == "" || hfhNamesStr == "" || hfhContentsStr == "" {
			continue
		}

		dirVector, err := hfh.HexSimhashToVector(hfhDirsStr, VectorDim)
		if err != nil {
			continue
		}
		nameVector, err := hfh.HexSimhashToVector(hfhNamesStr, VectorDim)
		if err != nil {
			continue
		}
		contentVector, err := hfh.HexSimhashToVector(hfhContentsStr, VectorDim)
		if err != nil {
			continue
		}

		// Parse metadata
		totalFiles, _ := strconv.ParseInt(record[11], 10, 32)
		indexedFiles, _ := strconv.ParseInt(record[12], 10, 32)
		sourceFiles, _ := strconv.ParseInt(record[13], 10, 32)
		ignoredFiles, _ := strconv.ParseInt(record[14], 10, 32)
		size, _ := strconv.ParseInt(record[15], 10, 32)
		categoryStr := strings.TrimSpace(record[16])

		// Generate point ID
		idStringToHash := urlHashStr + categoryStr + hfhDirsStr + hfhNamesStr + hfhContentsStr
		hasher := fnv.New64a()
		hasher.Write([]byte(idStringToHash))
		pointId := hasher.Sum64()

		// Common payload for all collections
		payload := map[string]*qdrant.Value{
			"vendor":        qdrant.NewValueString(strings.TrimSpace(record[4])),
			"component":     qdrant.NewValueString(strings.TrimSpace(record[5])),
			"version":       qdrant.NewValueString(strings.TrimSpace(record[6])),
			"release_date":  qdrant.NewValueString(strings.TrimSpace(record[7])),
			"license":       qdrant.NewValueString(strings.TrimSpace(record[8])),
			"purl":          qdrant.NewValueString(strings.TrimSpace(record[9])),
			"url":           qdrant.NewValueString(strings.TrimSpace(record[10])),
			"total_files":   qdrant.NewValueInt(totalFiles),
			"indexed_files": qdrant.NewValueInt(indexedFiles),
			"source_files":  qdrant.NewValueInt(sourceFiles),
			"ignored_files": qdrant.NewValueInt(ignoredFiles),
			"size":          qdrant.NewValueInt(size),
			"category":      qdrant.NewValueString(categoryStr),
		}

		// Parse language extensions if present (column 17)
		if len(record) > 17 && strings.TrimSpace(record[17]) != "" {
			langExtStr := strings.TrimSpace(record[17])

			var langExtensions map[string]int
			if err := json.Unmarshal([]byte(langExtStr), &langExtensions); err != nil {
				log.Printf("WARNING: Failed to parse language_extensions JSON '%s': %v. Storing as string instead.", langExtStr, err)
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
			}
		}

		// Create points for each collection
		dirsPoints = append(dirsPoints, &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointId),
			Vectors: qdrant.NewVectors(dirVector...),
			Payload: payload,
		})

		namesPoints = append(namesPoints, &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointId),
			Vectors: qdrant.NewVectors(nameVector...),
			Payload: payload,
		})

		contentsPoints = append(contentsPoints, &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointId),
			Vectors: qdrant.NewVectors(contentVector...),
			Payload: payload,
		})
	}

	if len(dirsPoints) == 0 {
		return nil
	}

	// Insert to all collections in parallel using goroutines
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// Insert dirs
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: DirsCollectionName,
			Points:         dirsPoints,
		})
		if err != nil {
			errChan <- fmt.Errorf("error inserting to dirs collection: %v", err)
		}
	}()

	// Insert names
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: NamesCollectionName,
			Points:         namesPoints,
		})
		if err != nil {
			errChan <- fmt.Errorf("error inserting to names collection: %v", err)
		}
	}()

	// Insert contents
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: ContentsCollectionName,
			Points:         contentsPoints,
		})
		if err != nil {
			errChan <- fmt.Errorf("error inserting to contents collection: %v", err)
		}
	}()

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

// Function to show collection statistics
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
