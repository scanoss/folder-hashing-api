package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

const (
	QdrantHost = "localhost"
	QdrantPort = 6334
	BatchSize  = 1000 // Smaller batch size for Qdrant
	MaxWorkers = 8
	VectorDim  = 64 // Single 64-bit hash per collection
)

// Collection names for multi-index approach
var (
	CollectionBaseName     = "url_collection"
	DirsCollectionName     = ""
	NamesCollectionName    = ""
	ContentsCollectionName = ""
)

func main() {
	// Process command line arguments
	csvDir := flag.String("dir", "", "Directory containing CSV files (required)")
	collectionNameFlag := flag.String("collectionName", "url_collection", "Name of the Qdrant collection to use")
	overwriteFlag := flag.Bool("overwrite", false, "If true, delete existing collection before import")

	flag.Parse()

	// Set collection names from flag
	CollectionBaseName = *collectionNameFlag
	DirsCollectionName = CollectionBaseName + "_dirs"
	NamesCollectionName = CollectionBaseName + "_names"
	ContentsCollectionName = CollectionBaseName + "_contents"

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

	// Check if collections exist and handle overwrite flag
	log.Println("Checking if collections exist...")
	dirsExists, err := client.CollectionExists(ctx, DirsCollectionName)
	if err != nil {
		log.Fatalf("Error checking if dirs collection exists: %v", err)
	}

	namesExists, err := client.CollectionExists(ctx, NamesCollectionName)
	if err != nil {
		log.Fatalf("Error checking if names collection exists: %v", err)
	}

	contentsExists, err := client.CollectionExists(ctx, ContentsCollectionName)
	if err != nil {
		log.Fatalf("Error checking if contents collection exists: %v", err)
	}

	if *overwriteFlag && (dirsExists || namesExists || contentsExists) {
		log.Printf("Collections exist and overwrite flag is set. Dropping collections...")
		if dirsExists {
			err = client.DeleteCollection(ctx, DirsCollectionName)
			if err != nil {
				log.Fatalf("Error dropping dirs collection: %v", err)
			}
			log.Printf("Collection '%s' dropped successfully", DirsCollectionName)
		}
		if namesExists {
			err = client.DeleteCollection(ctx, NamesCollectionName)
			if err != nil {
				log.Fatalf("Error dropping names collection: %v", err)
			}
			log.Printf("Collection '%s' dropped successfully", NamesCollectionName)
		}
		if contentsExists {
			err = client.DeleteCollection(ctx, ContentsCollectionName)
			if err != nil {
				log.Fatalf("Error dropping contents collection: %v", err)
			}
			log.Printf("Collection '%s' dropped successfully", ContentsCollectionName)
		}
		dirsExists = false
		namesExists = false
		contentsExists = false
	}

	// Create collections if they don't exist
	if !dirsExists || !namesExists || !contentsExists {
		createCollections(ctx, client)
	} else {
		log.Printf("Using existing collections: %s, %s, %s", DirsCollectionName, NamesCollectionName, ContentsCollectionName)
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
	showCollectionStats(ctx, client)
}

// Create collections if they don't exist
func createCollections(ctx context.Context, client *qdrant.Client) {
	log.Printf("Creating multi-index collections: %s, %s, %s", DirsCollectionName, NamesCollectionName, ContentsCollectionName)

	// Create dirs collection
	err := createSingleCollection(ctx, client, DirsCollectionName)
	if err != nil {
		log.Fatalf("Error creating dirs collection: %v", err)
	}

	// Create names collection
	err = createSingleCollection(ctx, client, NamesCollectionName)
	if err != nil {
		log.Fatalf("Error creating names collection: %v", err)
	}

	// Create contents collection
	err = createSingleCollection(ctx, client, ContentsCollectionName)
	if err != nil {
		log.Fatalf("Error creating contents collection: %v", err)
	}

	log.Printf("All collections created successfully")
}

// Create a single collection with vector configuration
func createSingleCollection(ctx context.Context, client *qdrant.Client, collectionName string) error {
	log.Printf("Creating collection '%s'...", collectionName)

	// Create collection with vector configuration
	err := client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     VectorDim,
			Distance: qdrant.Distance_Manhattan, // Use Manhattan distance for Hamming distance on binary vectors
		}),
		// Enable optimizers for better performance
		OptimizersConfig: &qdrant.OptimizersConfigDiff{
			DefaultSegmentNumber: qdrant.PtrOf(uint64(2)),
			MaxSegmentSize:       qdrant.PtrOf(uint64(20000)),
		},
	})
	if err != nil {
		return fmt.Errorf("error creating collection %s: %v", collectionName, err)
	}
	log.Printf("Collection '%s' created successfully", collectionName)
	return nil
}

// Import data from a CSV file
func importCSVFile(ctx context.Context, client *qdrant.Client, filePath, sectorName string) error {
	// Open the CSV file
	log.Printf("Opening CSV file: %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %v", err)
	}
	defer file.Close()

	// Read the CSV file line by line
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = 0 // Allow variable number of fields

	var validRecords [][]string
	var lineNumber int

	// Read records one by one
	for {
		lineNumber++
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break // End of file, exit loop
			}
			// Log warning about error in this line but continue
			log.Printf("WARNING: Error reading line %d in file %s: %v", lineNumber, filePath, err)
			continue
		}

		// Add valid record to our collection
		validRecords = append(validRecords, record)
	}

	totalRecords := len(validRecords)
	if totalRecords == 0 {
		log.Printf("No valid records found in %s after processing, skipping", filePath)
		return nil
	}

	log.Printf("Importing %d valid records from sector %s", totalRecords, sectorName)

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

		// Try to insert the batch, but handle errors
		err := insertBatch(ctx, client, batch)
		if err != nil {
			log.Printf("WARNING: Error inserting batch %d: %v. Continuing with next batch.", batchNum, err)
			continue
		}

		batchesProcessed++
	}

	log.Printf("All %d batches for sector %s imported successfully", batchesProcessed, sectorName)
	return nil
}

// hashToVector converts a single 64-bit hash into a 64-dimensional binary vector
func hashToVector(hash uint64) []float32 {
	vector := make([]float32, 64)
	for i := 0; i < 64; i++ {
		if (hash>>i)&1 == 1 {
			vector[i] = 1.0
		} else {
			vector[i] = 0.0
		}
	}
	return vector
}

// Insert a batch of records to all three collections
func insertBatch(ctx context.Context, client *qdrant.Client, batch [][]string) error {
	var points []*qdrant.PointStruct

	// Process each record in the batch
	for _, record := range batch {
		if len(record) < 18 { // Ensure record has all 18 fields (aligned with Milvus)
			log.Printf("WARNING: Skipping incomplete record with %d fields: %v", len(record), record)
			continue
		}

		// Parse hash values (following Milvus field structure)
		hfhDirsStr := strings.TrimSpace(record[0])
		hfhNamesStr := strings.TrimSpace(record[1])
		// Skip record[2] as in Milvus implementation
		hfhContentsStr := strings.TrimSpace(record[3])

		// Convert hexadecimal strings to uint64
		hfhDirHash, err := strconv.ParseUint(hfhDirsStr, 16, 64)
		if err != nil {
			log.Printf("WARNING: Skipping record with invalid dir hash '%s': %v", hfhDirsStr, err)
			continue
		}

		hfhNamesHash, err := strconv.ParseUint(hfhNamesStr, 16, 64)
		if err != nil {
			log.Printf("WARNING: Skipping record with invalid names hash '%s': %v", hfhNamesStr, err)
			continue
		}

		hfhContentsHash, err := strconv.ParseUint(hfhContentsStr, 16, 64)
		if err != nil {
			log.Printf("WARNING: Skipping record with invalid contents hash '%s': %v", hfhContentsStr, err)
			continue
		}

		// Create individual vectors for each hash type
		dirVector := hashToVector(hfhDirHash)
		nameVector := hashToVector(hfhNamesHash)
		contentVector := hashToVector(hfhContentsHash)

		// Parse urlHash from record[4] (aligned with Milvus)
		urlHashStr := strings.TrimSpace(record[4])
		urlHashUnsigned, err := strconv.ParseUint(urlHashStr, 16, 64)
		if err != nil {
			log.Printf("WARNING: Skipping record with invalid url hash '%s': %v", urlHashStr, err)
			continue
		}
		pointId := uint64(urlHashUnsigned)

		// Parse numeric fields (adjusted indices to match Milvus structure)
		totalFiles, _ := strconv.ParseInt(record[12], 10, 32)
		indexedFiles, _ := strconv.ParseInt(record[13], 10, 32)
		sourceFiles, _ := strconv.ParseInt(record[14], 10, 32)
		ignoredFiles, _ := strconv.ParseInt(record[15], 10, 32)
		size, _ := strconv.ParseInt(record[16], 10, 32)

		// Parse category field (from record[17] as in Milvus)
		categoryStr := strings.TrimSpace(record[17])
		category, _ := strconv.ParseInt(categoryStr, 10, 8)

		// Create payload with all metadata (adjusted field indices to match Milvus)
		payload := map[string]*qdrant.Value{
			"hfh_dirs_hash":     qdrant.NewValueString(hfhDirsStr),
			"hfh_names_hash":    qdrant.NewValueString(hfhNamesStr),
			"hfh_contents_hash": qdrant.NewValueString(hfhContentsStr),
			"url_hash":          qdrant.NewValueString(urlHashStr),
			"vendor":            qdrant.NewValueString(strings.TrimSpace(record[5])),
			"component":         qdrant.NewValueString(strings.TrimSpace(record[6])),
			"version":           qdrant.NewValueString(strings.TrimSpace(record[7])),
			"release_date":      qdrant.NewValueString(strings.TrimSpace(record[8])),
			"license":           qdrant.NewValueString(strings.TrimSpace(record[9])),
			"purl":              qdrant.NewValueString(strings.TrimSpace(record[10])),
			"url":               qdrant.NewValueString(strings.TrimSpace(record[11])),
			"total_files":       qdrant.NewValueInt(totalFiles),
			"indexed_files":     qdrant.NewValueInt(indexedFiles),
			"source_files":      qdrant.NewValueInt(sourceFiles),
			"ignored_files":     qdrant.NewValueInt(ignoredFiles),
			"size":              qdrant.NewValueInt(size),
			"category":          qdrant.NewValueInt(category),
		}

		// Create points for each collection with same ID and payload but different vectors
		dirPoint := &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointId),
			Vectors: qdrant.NewVectors(dirVector...),
			Payload: payload,
		}

		namePoint := &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointId),
			Vectors: qdrant.NewVectors(nameVector...),
			Payload: payload,
		}

		contentPoint := &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointId),
			Vectors: qdrant.NewVectors(contentVector...),
			Payload: payload,
		}

		// Store points for batch insertion (we'll create separate batches for each collection)
		points = append(points, dirPoint, namePoint, contentPoint)
	}

	if len(points) == 0 {
		log.Printf("WARNING: No valid points to insert in this batch")
		return nil
	}

	// Separate points into three collections (every 3rd point goes to a different collection)
	var dirPoints, namePoints, contentPoints []*qdrant.PointStruct
	for i := 0; i < len(points); i += 3 {
		if i < len(points) {
			dirPoints = append(dirPoints, points[i])
		}
		if i+1 < len(points) {
			namePoints = append(namePoints, points[i+1])
		}
		if i+2 < len(points) {
			contentPoints = append(contentPoints, points[i+2])
		}
	}

	log.Printf("Inserting %d points to each collection", len(dirPoints))

	// Insert to dirs collection
	_, err := client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: DirsCollectionName,
		Points:         dirPoints,
	})
	if err != nil {
		return fmt.Errorf("error inserting to dirs collection: %v", err)
	}

	// Insert to names collection
	_, err = client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: NamesCollectionName,
		Points:         namePoints,
	})
	if err != nil {
		return fmt.Errorf("error inserting to names collection: %v", err)
	}

	// Insert to contents collection
	_, err = client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: ContentsCollectionName,
		Points:         contentPoints,
	})
	if err != nil {
		return fmt.Errorf("error inserting to contents collection: %v", err)
	}

	return nil
}

// Function to show collection statistics for all three collections
func showCollectionStats(ctx context.Context, client *qdrant.Client) {
	collections := map[string]string{
		"Directories": DirsCollectionName,
		"Names":       NamesCollectionName,
		"Contents":    ContentsCollectionName,
	}

	log.Println("Retrieving collection information for all collections...")

	for collectionType, collectionName := range collections {
		log.Printf("\n=== %s Collection (%s) ===", collectionType, collectionName)

		info, err := client.GetCollectionInfo(ctx, collectionName)
		if err != nil {
			log.Printf("Could not retrieve %s collection information: %v", collectionType, err)
			continue
		}

		log.Printf("  Status: %s", info.Status)

		// Access vector configuration if available
		if info.Config != nil && info.Config.Params != nil {
			log.Printf("  Vector configuration available")
		}

		log.Printf("  Points count: %d", info.PointsCount)
		log.Printf("  Segments count: %d", info.SegmentsCount)

		// Try to sample a few points to verify they exist
		log.Printf("Attempting to scroll through first few points in %s collection...", collectionType)
		scrollResp, err := client.Scroll(ctx, &qdrant.ScrollPoints{
			CollectionName: collectionName,
			Limit:          qdrant.PtrOf(uint32(3)), // Get first 3 points
			WithPayload:    qdrant.NewWithPayload(true),
			WithVectors:    qdrant.NewWithVectors(false), // Don't need vectors for this check
		})
		if err != nil {
			log.Printf("ERROR: Could not scroll points in %s collection: %v", collectionType, err)
		} else {
			log.Printf("  Sample points found: %d", len(scrollResp))
			for i, point := range scrollResp {
				log.Printf("    Point %d: ID=%v", i+1, point.Id)
				if point.Payload != nil {
					if vendor, exists := point.Payload["vendor"]; exists {
						log.Printf("      Vendor: %v", vendor.GetStringValue())
					}
					if component, exists := point.Payload["component"]; exists {
						log.Printf("      Component: %v", component.GetStringValue())
					}
				}
			}
		}
	}
}
