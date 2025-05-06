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
var CollectionName = "url"

func main() {
	// Process command line arguments
	csvDir := flag.String("dir", "", "Directory containing CSV files (required)")
	collectionNameFlag := flag.String("collectionName", "url", "Name of the Milvus collection to use")
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

	// Handle database creation/selection
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
// s.Metadata.SrcHash, s.Metadata.Vendor, s.Metadata.Component, s.Metadata.Version, s.Metadata.Release_date, s.Metadata.License, s.Metadata.Purl, s.Metadata.Url, len(s.Metadata.FilesItems), s.Metadata.IndexedFiles, s.Metadata.SourceFiles, s.Metadata.IgnoredFiles, s.Metadata.Size)

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
		log.Println("Collection created successfully")
	} else {
		log.Println("Using existing collection")
	}
}

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

	// Create the Index object using the correct signature
	idx = entity.NewGenericIndex("urlHashIndex", entity.AUTOINDEX, nil)

	// Try to create the index with this type
	err = c.CreateIndex(ctx, CollectionName, "urlHash", idx, true, opts...)

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
	urlHash := make([]int64, len(batch))
	vendor := make([]string, len(batch))
	component := make([]string, len(batch))
	version := make([]string, len(batch))
	release_date := make([]string, len(batch))
	license := make([]string, len(batch))
	purl := make([]string, len(batch))
	url := make([]string, len(batch))
	total_files := make([]int32, len(batch))
	indexed_files := make([]int32, len(batch))
	source_files := make([]int32, len(batch))
	ignored_files := make([]int32, len(batch))
	size := make([]int32, len(batch))

	for j, record := range batch {
		if len(record) < 3 {
			return fmt.Errorf("incomplete record at position %d: %v", j, record)
		}

		hfhNamesStr := strings.TrimSpace(record[0])
		hfhContentsStr := strings.TrimSpace(record[1])
		// Convert hexadecimal strings to uint64
		hfhNamesHash, err := strconv.ParseUint(hfhNamesStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal hash '%s': %v", hfhNamesStr, err)
		}

		hfhContentsHash, err := strconv.ParseUint(hfhContentsStr, 16, 64)
		if err != nil {
			return fmt.Errorf("error parsing hexadecimal data1 '%s': %v", hfhContentsStr, err)
		}
		hfhNamesBin := make([]byte, 8)
		binary.BigEndian.PutUint64(hfhNamesBin, hfhNamesHash)

		hfhContentsBin := make([]byte, 8)
		binary.BigEndian.PutUint64(hfhContentsBin, hfhContentsHash)
		hfhNames[j] = hfhNamesBin
		hfhContents[j] = hfhContentsBin

		hashStr := strings.TrimSpace(record[2])
		hashUnsigned, _ := strconv.ParseUint(hashStr, 16, 64)
		urlHash[j] = int64(hashUnsigned)
		vendor[j] = strings.TrimSpace(record[3])
		component[j] = strings.TrimSpace(record[4])
		version[j] = strings.TrimSpace(record[5])
		release_date[j] = strings.TrimSpace(record[6])
		license[j] = strings.TrimSpace(record[7])
		purl[j] = strings.TrimSpace(record[8])
		url[j] = strings.TrimSpace(record[9])

		num, _ := strconv.ParseInt(record[10], 10, 32)
		total_files[j] = int32(num)

		num, _ = strconv.ParseInt(record[11], 10, 32)
		indexed_files[j] = int32(num)

		num, _ = strconv.ParseInt(record[12], 10, 32)
		source_files[j] = int32(num)

		num, _ = strconv.ParseInt(record[13], 10, 32)
		ignored_files[j] = int32(num)

		num, _ = strconv.ParseInt(record[14], 10, 32)
		size[j] = int32(num)

		// Verification for the first record of each batch
		if j == 0 {
			log.Printf("Example: %x, %x, url: %s, purl: %s\n",
				uint64(urlHash[j]), hfhNames[j], url[j], purl[j])
		}
	}

	// Create columns for insertion
	hashNamesColumn := entity.NewColumnBinaryVector("hfhNames", 64, hfhNames) // 64-bit vector
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

	// Insert data
	//_, err := c.Insert(ctx, CollectionName, sectorName, hashNamesColumn, hashContentsColumn, urlColumn)
	_, err := c.Upsert(ctx, CollectionName, sectorName, hashNamesColumn, hashContentsColumn, urlHashColumn, vendorColumn, componentColumn, versionColumn,
		release_dateColumn, licenseColumn, purlColumn, urlColumn, total_filesColumn, indexed_filesColumn,
		source_filesColumn, ignored_filesColumn, sizeColumn)

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
