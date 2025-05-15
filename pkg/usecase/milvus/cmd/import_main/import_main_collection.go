package main

import (
	"context"
	"encoding/binary"
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

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	MilvusHost = "localhost"
	MilvusPort = "19530"
	BatchSize  = 4000
	MaxWorkers = 3
)

// Default value for collection name, can be overridden with -collectionName flag
var CollectionName = "url"

func main() {
	// Process command line arguments
	csvDir := flag.String("dir", "", "Directory containing CSV files (required)")
	collectionNameFlag := flag.String("collectionName", "url", "Name of the Milvus collection to use")
	databaseNameFlag := flag.String("database", "default", "Name of the Milvus database to use (will be created if it doesn't exist)")
	overwriteFlag := flag.Bool("overwrite", false, "If true, delete existing collection before import")

	flag.Parse()

	// Set collection name from flag
	CollectionName = *collectionNameFlag

	if *csvDir == "" {
		log.Fatal("Error: You must specify a directory with the -dir option")
	}

	// Verify that the directory exists
	if _, err := os.Stat(*csvDir); os.IsNotExist(err) {
		log.Fatalf("Error: Directory %s does not exist", *csvDir)
	}

	// Start the timer to measure performance
	startTime := time.Now()

	// Connect to Milvus
	log.Println("Connecting to Milvus server...")
	ctx := context.Background()
	c, err := client.NewGrpcClient(ctx, fmt.Sprintf("%s:%s", MilvusHost, MilvusPort))
	if err != nil {
		log.Fatalf("Error connecting to Milvus: %v", err)
	}
	defer func() {
		log.Println("Closing connection to Milvus")
		c.Close()
	}()
	log.Println("Connected to Milvus server successfully")

	// Handle database creation/selection
	databaseName := *databaseNameFlag
	if databaseName != "default" {
		// Check if database exists
		log.Printf("Checking if database '%s' exists...", databaseName)
		databases, err := c.ListDatabases(ctx)
		if err != nil {
			log.Fatalf("Error listing databases: %v", err)
		}

		databaseExists := false
		for _, db := range databases {
			if db.Name == databaseName {
				databaseExists = true
				break
			}
		}

		// Create database if it doesn't exist
		if !databaseExists {
			log.Printf("Database '%s' does not exist. Creating...", databaseName)
			err = c.CreateDatabase(ctx, databaseName)
			if err != nil {
				log.Fatalf("Error creating database '%s': %v", databaseName, err)
			}
			log.Printf("Database '%s' created successfully", databaseName)
		} else {
			log.Printf("Database '%s' already exists", databaseName)
		}

		// Use the specified database
		log.Printf("Switching to database: %s", databaseName)
		err = c.UsingDatabase(ctx, databaseName)
		if err != nil {
			log.Fatalf("Error setting active database to '%s': %v", databaseName, err)
		}
		log.Printf("Now using database: %s", databaseName)
	}
	// Check if collection exists and handle overwrite flag
	hasCollection, err := c.HasCollection(ctx, CollectionName)
	if err != nil {
		log.Fatalf("Error checking if collection exists: %v", err)
	}

	if hasCollection && *overwriteFlag {
		log.Printf("Collection '%s' exists and overwrite flag is set. Dropping collection...", CollectionName)
		err = c.DropCollection(ctx, CollectionName)
		if err != nil {
			log.Fatalf("Error dropping collection: %v", err)
		}
		log.Printf("Collection '%s' dropped successfully", CollectionName)
		hasCollection = false
	}

	log.Println("Checking if collection exists...")
	// Create collection if it doesn't exist
	createCollectionIfNotExists(ctx, c)

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

	// Flush after importing each sector to ensure data is persisted
	log.Printf("Flushing data...")
	err = c.Flush(ctx, CollectionName, false)
	if err != nil {
		log.Printf("error flushing data: %v", err)
	}
	log.Printf("Data flushed successfully")

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

				err := importCSVFile(ctx, c, file, sectorName)
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

	// Create search indices synchronously
	log.Println("Creating search indices (synchronous operation)...")
	err = createSearchIndices(ctx, c)
	if err != nil {
		log.Printf("WARNING: Error creating search indices: %v", err)
	} else {
		log.Println("Search indices created successfully")
	}

	// Load collection to make it available for search
	log.Println("Loading collection to make it available for search...")
	err = c.LoadCollection(ctx, CollectionName, false)
	if err != nil {
		log.Printf("WARNING: Error loading collection: %v", err)
	} else {
		log.Println("Collection loaded successfully")
	}

	// Show statistics
	elapsed := time.Since(startTime)
	log.Printf("Import process completed. Total time: %s", elapsed)

	// Show collection statistics if possible
	showCollectionStats(ctx, c)
}

// Create collection if it doesn't exist
func createCollectionIfNotExists(ctx context.Context, c client.Client) {
	has, err := c.HasCollection(ctx, CollectionName)
	if err != nil {
		log.Fatalf("Error verifying collection existence: %v", err)
	}

	if !has {
		log.Printf("Collection '%s' does not exist. Creating new collection...", CollectionName)

		// Define collection fields
		fields := []*entity.Field{
			{
				Name:     "hfhNames",
				DataType: entity.FieldTypeBinaryVector,
				TypeParams: map[string]string{
					"dim": "64", // 64 bits for uint64
				},
			},
			{
				Name:     "hfhContents",
				DataType: entity.FieldTypeBinaryVector, // For uint64
				TypeParams: map[string]string{
					"dim": "64", // 64 bits for uint64
				},
			},
			{
				Name:       "urlHash",
				DataType:   entity.FieldTypeInt64,
				TypeParams: map[string]string{},
				PrimaryKey: true,
				AutoID:     false,
			},
			{
				Name:     "vendor",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "1024",
				},
			},
			{
				Name:     "component",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "256",
				},
			},
			{
				Name:     "version",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "256",
				},
			},
			{
				Name:     "release_date",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "16",
				},
			},
			{
				Name:     "license",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "512",
				},
			},
			{
				Name:     "purl",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "512",
				},
			},
			{
				Name:     "url",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "2048",
				},
			},
			{
				Name:     "total_files",
				DataType: entity.FieldTypeInt32,
			},
			{
				Name:     "indexed_files",
				DataType: entity.FieldTypeInt32,
			},
			{
				Name:     "source_files",
				DataType: entity.FieldTypeInt32,
			},
			{
				Name:     "ignored_files",
				DataType: entity.FieldTypeInt32,
			},
			{
				Name:     "size",
				DataType: entity.FieldTypeInt32,
			},
		}

		// Create the collection
		schema := &entity.Schema{
			CollectionName: CollectionName,
			Description:    "Collection for proximity hash search",
			Fields:         fields,
		}

		err = c.CreateCollection(ctx, schema, 1)
		if err != nil {
			log.Fatalf("Error creating collection: %v", err)
		}
		log.Printf("Collection '%s' created successfully", CollectionName)
	} else {
		log.Printf("Using existing collection '%s'", CollectionName)
	}
}

// Create search indices synchronously
func createSearchIndices(ctx context.Context, c client.Client) error {
	// Verify collection
	has, err := c.HasCollection(ctx, CollectionName)
	if err != nil || !has {
		return fmt.Errorf("error verifying collection: %v", err)
	}

	// Create indices one by one, synchronously
	// Create hfhNames index
	log.Println("Creating index for hfhNames (synchronous)...")
	idx := entity.NewGenericIndex("hfhNameIndex", entity.AUTOINDEX, nil)
	var opts []client.IndexOption
	err = c.CreateIndex(ctx, CollectionName, "hfhNames", idx, true, opts...) // false = not async
	if err != nil {
		return fmt.Errorf("error creating hfhNames index: %v", err)
	}
	log.Println("hfhNames index created successfully")

	// Create hfhContents index
	log.Println("Creating index for hfhContents (synchronous)...")
	idx = entity.NewGenericIndex("hfhContentsIndex", entity.AUTOINDEX, nil)
	err = c.CreateIndex(ctx, CollectionName, "hfhContents", idx, true, opts...) // false = not async
	if err != nil {
		return fmt.Errorf("error creating hfhContents index: %v", err)
	}
	log.Println("hfhContents index created successfully")

	// Create urlHash index
	log.Println("Creating index for urlHash (synchronous)...")
	idx = entity.NewGenericIndex("urlHashIndex", entity.AUTOINDEX, nil)
	err = c.CreateIndex(ctx, CollectionName, "urlHash", idx, true, opts...) // false = not async
	if err != nil {
		return fmt.Errorf("error creating urlHash index: %v", err)
	}
	log.Println("urlHash index created successfully")

	return nil
}

// Import data from a CSV file
func importCSVFile(ctx context.Context, c client.Client, filePath, sectorName string) error {
	// Create partition if it doesn't exist
	err := createPartitionIfNotExists(ctx, c, sectorName)
	if err != nil {
		return fmt.Errorf("error creating partition for %s: %v", sectorName, err)
	}

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
		err := insertBatch(ctx, c, batch, sectorName)
		if err != nil {
			log.Printf("WARNING: Error inserting batch %d: %v. Continuing with next batch.", batchNum, err)
			continue
		}

		batchesProcessed++
	}

	// Flush after importing each sector to ensure data is persisted
	log.Printf("Flushing data for sector %s...", sectorName)
	err = c.Flush(ctx, CollectionName, true)
	if err != nil {
		return fmt.Errorf("error flushing data for sector %s: %v", sectorName, err)
	}

	log.Printf("Data for sector %s flushed successfully", sectorName)
	log.Printf("All %d batches for sector %s imported successfully", batchesProcessed, sectorName)
	return nil
}

// Insert a batch of records
func insertBatch(ctx context.Context, c client.Client, batch [][]string, sectorName string) error {
	// Prepare columns for insertion
	recordCount := len(batch)
	hfhNames := make([][]byte, recordCount)
	hfhContents := make([][]byte, recordCount)
	urlHash := make([]int64, recordCount)
	vendor := make([]string, recordCount)
	component := make([]string, recordCount)
	version := make([]string, recordCount)
	release_date := make([]string, recordCount)
	license := make([]string, recordCount)
	purl := make([]string, recordCount)
	url := make([]string, recordCount)
	total_files := make([]int32, recordCount)
	indexed_files := make([]int32, recordCount)
	source_files := make([]int32, recordCount)
	ignored_files := make([]int32, recordCount)
	size := make([]int32, recordCount)

	for j, record := range batch {
		// Validate record has sufficient fields
		if len(record) < 15 {
			return fmt.Errorf("incomplete record at position %d: expected at least 15 fields, got %d", j, len(record))
		}

		// Process hfhNames (field 0)
		hfhNamesStr := strings.TrimSpace(record[0])
		hfhNamesHash, err := strconv.ParseUint(hfhNamesStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal hash '%s' at position %d: %v", hfhNamesStr, j, err)
		}
		hfhNamesBin := make([]byte, 8)
		binary.BigEndian.PutUint64(hfhNamesBin, hfhNamesHash)
		hfhNames[j] = hfhNamesBin

		// Process hfhContents (field 1)
		hfhContentsStr := strings.TrimSpace(record[1])
		hfhContentsHash, err := strconv.ParseUint(hfhContentsStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal hash '%s' at position %d: %v", hfhContentsStr, j, err)
		}
		hfhContentsBin := make([]byte, 8)
		binary.BigEndian.PutUint64(hfhContentsBin, hfhContentsHash)
		hfhContents[j] = hfhContentsBin

		// Process urlHash (field 2)
		hashStr := strings.TrimSpace(record[2])
		hashUnsigned, err := strconv.ParseUint(hashStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal url hash '%s' at position %d: %v", hashStr, j, err)
		}
		urlHash[j] = int64(hashUnsigned)

		// Process remaining string fields
		vendor[j] = strings.TrimSpace(record[3])
		component[j] = strings.TrimSpace(record[4])
		version[j] = strings.TrimSpace(record[5])
		release_date[j] = strings.TrimSpace(record[6])
		license[j] = strings.TrimSpace(record[7])
		purl[j] = strings.TrimSpace(record[8])
		url[j] = strings.TrimSpace(record[9])

		// Process numeric fields with better error handling
		var parseIntErr error
		var num int64

		// Process total_files (field 10)
		num, parseIntErr = strconv.ParseInt(strings.TrimSpace(record[10]), 10, 32)
		if parseIntErr != nil {
			log.Printf("Warning: Cannot parse total_files '%s' at position %d: %v. Setting to 0.", record[10], j, parseIntErr)
			num = 0
		}
		total_files[j] = int32(num)

		// Process indexed_files (field 11)
		num, parseIntErr = strconv.ParseInt(strings.TrimSpace(record[11]), 10, 32)
		if parseIntErr != nil {
			log.Printf("Warning: Cannot parse indexed_files '%s' at position %d: %v. Setting to 0.", record[11], j, parseIntErr)
			num = 0
		}
		indexed_files[j] = int32(num)

		// Process source_files (field 12)
		num, parseIntErr = strconv.ParseInt(strings.TrimSpace(record[12]), 10, 32)
		if parseIntErr != nil {
			log.Printf("Warning: Cannot parse source_files '%s' at position %d: %v. Setting to 0.", record[12], j, parseIntErr)
			num = 0
		}
		source_files[j] = int32(num)

		// Process ignored_files (field 13)
		num, parseIntErr = strconv.ParseInt(strings.TrimSpace(record[13]), 10, 32)
		if parseIntErr != nil {
			log.Printf("Warning: Cannot parse ignored_files '%s' at position %d: %v. Setting to 0.", record[13], j, parseIntErr)
			num = 0
		}
		ignored_files[j] = int32(num)

		// Process size (field 14)
		num, parseIntErr = strconv.ParseInt(strings.TrimSpace(record[14]), 10, 32)
		if parseIntErr != nil {
			log.Printf("Warning: Cannot parse size '%s' at position %d: %v. Setting to 0.", record[14], j, parseIntErr)
			num = 0
		}
		size[j] = int32(num)

		// Log first record of each batch for verification
		if j == 0 {
			log.Printf("Example record: hfhNames: %x, hfhContents: %x, urlHash: %x, url: %s, purl: %s",
				hfhNamesHash, hfhContentsHash, hashUnsigned, url[j], purl[j])
		}
	}

	// Create columns for insertion
	hashNamesColumn := entity.NewColumnBinaryVector("hfhNames", 64, hfhNames)
	hashContentsColumn := entity.NewColumnBinaryVector("hfhContents", 64, hfhContents)
	urlHashColumn := entity.NewColumnInt64("urlHash", urlHash)
	vendorColumn := entity.NewColumnVarChar("vendor", vendor)
	componentColumn := entity.NewColumnVarChar("component", component)
	versionColumn := entity.NewColumnVarChar("version", version)
	release_dateColumn := entity.NewColumnVarChar("release_date", release_date)
	licenseColumn := entity.NewColumnVarChar("license", license)
	purlColumn := entity.NewColumnVarChar("purl", purl)
	urlColumn := entity.NewColumnVarChar("url", url)
	total_filesColumn := entity.NewColumnInt32("total_files", total_files)
	indexed_filesColumn := entity.NewColumnInt32("indexed_files", indexed_files)
	source_filesColumn := entity.NewColumnInt32("source_files", source_files)
	ignored_filesColumn := entity.NewColumnInt32("ignored_files", ignored_files)
	sizeColumn := entity.NewColumnInt32("size", size)

	// Insert data using Upsert
	log.Printf("Upserting %d records to collection '%s', partition '%s'...", recordCount, CollectionName, sectorName)
	_, err := c.Upsert(ctx, CollectionName, sectorName,
		hashNamesColumn,
		hashContentsColumn,
		urlHashColumn,
		vendorColumn,
		componentColumn,
		versionColumn,
		release_dateColumn,
		licenseColumn,
		purlColumn,
		urlColumn,
		total_filesColumn,
		indexed_filesColumn,
		source_filesColumn,
		ignored_filesColumn,
		sizeColumn)

	if err != nil {
		return fmt.Errorf("error upserting data: %v", err)
	}

	log.Printf("Successfully upserted %d records", recordCount)
	return nil
}

func createPartitionIfNotExists(ctx context.Context, c client.Client, partitionName string) error {
	has, err := c.HasPartition(ctx, CollectionName, partitionName)
	if err != nil {
		return fmt.Errorf("error checking partition existence: %v", err)
	}

	if !has {
		log.Printf("Creating new partition: %s", partitionName)
		err = c.CreatePartition(ctx, CollectionName, partitionName)
		if err != nil {
			return fmt.Errorf("error creating partition: %v", err)
		}
		log.Printf("Partition %s created successfully", partitionName)
	} else {
		log.Printf("Using existing partition: %s", partitionName)
	}

	return nil
}

// Function to show collection statistics
func showCollectionStats(ctx context.Context, c client.Client) {
	// Try to get collection statistics
	log.Println("Retrieving collection statistics...")
	stats, err := c.GetCollectionStatistics(ctx, CollectionName)
	if err != nil {
		log.Printf("Could not retrieve collection statistics: %v", err)
		return
	}

	// Extract row count from statistics
	rowCount, ok := stats["row_count"]
	if !ok {
		log.Println("Row count not available in collection statistics")
		return
	}

	log.Printf("Collection '%s' statistics: %v", CollectionName, stats)
	log.Printf("Total rows in collection: %s", rowCount)

	// Try to list all partitions
	partitions, err := c.ShowPartitions(ctx, CollectionName)
	if err != nil {
		log.Printf("Could not list partitions: %v", err)
		return
	}

	log.Printf("Collection has %d partitions:", len(partitions))
	for i, partition := range partitions {
		log.Printf("  Partition %d: %s", i+1, partition)
	}
}
