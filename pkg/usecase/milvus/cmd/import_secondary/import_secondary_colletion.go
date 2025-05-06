package main

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"io/ioutil"
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
	BatchSize  = 1000
	MaxWorkers = 8
)

// Default value for collection name, can be overridden with -collectionName flag
var CollectionName = "secondary"

func main() {
	// Process command line arguments
	csvDir := flag.String("dir", "", "Directory containing CSV files (required)")
	collectionNameFlag := flag.String("collectionName", "secondary", "Name of the Milvus collection to use")
	overwriteFlag := flag.Bool("overwrite", false, "If true, delete existing collection before import")
	databaseNameFlag := flag.String("database", "default", "Name of the Milvus database to use (will be created if it doesn't exist)")

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
	ctx := context.Background()
	c, err := client.NewGrpcClient(ctx, fmt.Sprintf("%s:%s", MilvusHost, MilvusPort))
	if err != nil {
		log.Fatalf("Error connecting to Milvus: %v", err)
	}
	defer c.Close()

	//Handle database creation/selection
	databaseName := *databaseNameFlag
	if databaseName != "default" {
		// Check if database exists
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
		}

		// Use the specified database
		log.Printf("Using database: %s", databaseName)
		err = c.UsingDatabase(ctx, databaseName)
		if err != nil {
			log.Fatalf("Error setting active database to '%s': %v", databaseName, err)
		}
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

	log.Println("Creating Collection")
	// Create collection if it doesn't exist
	createCollectionIfNotExists(ctx, c)

	// Get list of CSV files in the directory
	files, err := ioutil.ReadDir(*csvDir)
	if err != nil {
		log.Fatalf("Error reading directory: %v", err)
	}
	log.Println("Searching for CSV files")

	var csvFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".csv") {
			csvFiles = append(csvFiles, filepath.Join(*csvDir, file.Name()))
		}
	}

	log.Printf("Found %d CSV files to import", len(csvFiles))

	// Channel to process files
	filesChan := make(chan string, len(csvFiles))
	var wg sync.WaitGroup

	// Start workers to process files in parallel
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
					log.Printf("Error importing file %s: %v", file, err)
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
	wg.Wait()

	// Flush to ensure all data is available
	err = c.Flush(ctx, CollectionName, false)
	if err != nil {
		log.Printf("Warning: Error flushing data: %v", err)
	}

	// Create search index if it doesn't exist yet
	createSearchIndex(ctx, c)

	// Show statistics
	elapsed := time.Since(startTime)
	log.Printf("Import completed. Total time: %s", elapsed)
}

// Create collection if it doesn't exist
func createCollectionIfNotExists(ctx context.Context, c client.Client) {
	has, err := c.HasCollection(ctx, CollectionName)
	if err != nil {
		log.Fatalf("Error verifying collection existence: %v", err)
	}

	if !has {
		log.Println("Creating new collection...")

		// Define collection fields
		fields := []*entity.Field{
			{
				Name:       "id",
				DataType:   entity.FieldTypeInt64,
				PrimaryKey: true,
				AutoID:     true,
			},
			{
				Name:     "hfhContents",
				DataType: entity.FieldTypeBinaryVector,
				TypeParams: map[string]string{
					"dim": "64", // 64 bits for uint64
				},
			},
			{
				Name:     "hfhNames",
				DataType: entity.FieldTypeBinaryVector, // For uint64
				TypeParams: map[string]string{
					"dim": "64", // 64 bits for uint64
				},
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
		log.Println("Collection created successfully")
	} else {
		log.Println("Using existing collection")
	}
}

// Create search index
func createSearchIndex(ctx context.Context, c client.Client) {
	// Verify collection
	has, err := c.HasCollection(ctx, CollectionName)
	if err != nil || !has {
		log.Printf("Error verifying collection: %v", err)
		return
	}
	log.Println("Creating index for fast searches...")

	// Create the Index object using the correct signature
	idx := entity.NewGenericIndex("hfhNameIndex", entity.AUTOINDEX, nil)

	// Index options (empty slice if no specific options)
	var opts []client.IndexOption

	// Try to create the index with this type
	err = c.CreateIndex(ctx, CollectionName, "hfhNames", idx, true, opts...)

	if err != nil {
		log.Println("Error creating index: %v", err)
	}

	// Create the Index object using the correct signature
	idx = entity.NewGenericIndex("hfhContentsIndex", entity.AUTOINDEX, nil)

	// Try to create the index with this type
	err = c.CreateIndex(ctx, CollectionName, "hfhContents", idx, true, opts...)

	if err != nil {
		log.Println("Error creating index: %v", err)
	}
}

// Import data from a CSV file
func importCSVFile(ctx context.Context, c client.Client, filePath, sectorName string) error {

	err := createPartitionIfNotExists(ctx, c, sectorName)
	if err != nil {
		return fmt.Errorf("error creating partition for %s: %v", sectorName, err)
	}

	// Open the CSV file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %v", err)
	}
	defer file.Close()

	// Read the CSV file
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = 0 // HASH, DATA1, DATA2

	// Read all lines
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("error reading CSV: %v", err)
	}

	totalRecords := len(records)
	if totalRecords == 0 {
		log.Printf("File %s is empty, skipping", filePath)
		return nil
	}

	log.Printf("Importing %d records from sector %s", totalRecords, sectorName)

	// Process in batches for better performance
	for i := 0; i < totalRecords; i += BatchSize {
		end := i + BatchSize
		if end > totalRecords {
			end = totalRecords
		}

		batch := records[i:end]
		err := insertBatch(ctx, c, batch, sectorName)
		if err != nil {
			return fmt.Errorf("error inserting batch %d: %v", i/BatchSize+1, err)
		}
	}

	log.Printf("Sector %s imported successfully", sectorName)
	return nil
}

// Insert a batch of records
func insertBatch(ctx context.Context, c client.Client, batch [][]string, sectorName string) error {
	// Prepare columns for insertion
	hfhNames := make([][]byte, len(batch))
	hfhContents := make([][]byte, len(batch))

	for j, record := range batch {
		if len(record) < 2 {
			return fmt.Errorf("incomplete record at position %d: %v", j, record)
		}

		hfhContentsStr := strings.TrimSpace(record[0])
		hfhNamesStr := strings.TrimSpace(record[1])

		// Convert hexadecimal strings to uint64
		hfhNamesHash, err := strconv.ParseUint(hfhNamesStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal hash '%s': %v", hfhNamesStr, err)
		}

		hfhContentsHash, err := strconv.ParseUint(hfhContentsStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal data1 '%s': %v", hfhContentsStr, err)
		}

		// Convert the hash to a 64-bit binary vector
		hfhNamesBin := make([]byte, 8)
		binary.BigEndian.PutUint64(hfhNamesBin, hfhNamesHash)

		hfhContentsBin := make([]byte, 8)
		binary.BigEndian.PutUint64(hfhContentsBin, hfhContentsHash)

		// Store data
		hfhNames[j] = hfhNamesBin
		hfhContents[j] = hfhContentsBin
	}

	// Create columns for insertion
	hashNamesColumn := entity.NewColumnBinaryVector("hfhNames", 64, hfhNames) // 64-bit vector
	hashContentsColumn := entity.NewColumnBinaryVector("hfhContents", 64, hfhContents)

	// Insert data
	_, err := c.Insert(ctx, CollectionName, sectorName, hashContentsColumn, hashNamesColumn)
	if err != nil {
		return fmt.Errorf("error inserting data: %v", err)
	}

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
