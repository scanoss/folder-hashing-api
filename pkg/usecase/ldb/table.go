package ldb

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	ldbBinaryPathDefault = "/usr/bin/ldb"
	kbDefaultPath        = "/var/lib/ldb/"
	ldbDefaultKb         = "oss"
)

// TableDefinition defines the base structure of a table
type TableDefinition struct {
	KbName        string
	TableName     string
	HashSize      int            // Hash size in bytes
	KeysNumber    int            // Number of keys (1 primary + N secondary)
	Fields        map[string]int // Table fields
	Definitions   int
	recordSize    int //0 means variable size
	cached        bool
	ts_ln         uint32
	path          string
	ldbBinaryPath string
	cache         [256]map[string][][]string
}

// NewTable creates a new table definition with default values
func NewTable(binaryPath string, kbName string, tableName string, hashSize int, recordSize int, keysNumber int, fields []string, definitions int, cached bool, data [][]string) *TableDefinition {

	if binaryPath == "" {
		binaryPath = ldbBinaryPathDefault
	}

	if kbName == "" {
		kbName = ldbDefaultKb
	}

	tablePath := LdbDefaultPath + "/" + kbName + "/" + tableName

	cols := make(map[string]int)
	for i, f := range fields {
		cols[f] = i
	}
	return &TableDefinition{
		KbName:        kbName,
		TableName:     tableName,
		HashSize:      hashSize,   // Default 16 bytes
		KeysNumber:    keysNumber, // Default 1 key
		Fields:        cols,       // Default data field only
		Definitions:   definitions,
		recordSize:    recordSize,
		cached:        cached,
		ts_ln:         2,
		path:          tablePath,
		ldbBinaryPath: binaryPath,
	}
}

// NewTableFromCfg creates a new table from the configuration files
func NewTableFromCfg(binaryPath string, kbName string, tableName string, fields []string, cached bool) (*TableDefinition, error) {

	if binaryPath == "" {
		binaryPath = ldbBinaryPathDefault
	}

	if kbName == "" {
		kbName = ldbDefaultKb
	}

	if tableName == "" {
		return nil, fmt.Errorf("must provide a table name")
	}

	//Build the path to the configuration file
	configPath := filepath.Join(kbDefaultPath, kbName, tableName+".cfg")

	//Check if the config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s does not exist", configPath)
	}

	//Open the config file
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open the config file %s %v", configPath, err)
	}
	defer file.Close()
	//Read the file in csv format
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = 4

	record, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to parse the CSV file: %v", err)
	}
	//convert each CSV field to the interger number.
	hashSize, err := strconv.Atoi(record[0])
	if err != nil {
		return nil, fmt.Errorf("failed to convert hashSize: %v", err)
	}

	recordSize, err := strconv.Atoi(record[1])
	if err != nil {
		return nil, fmt.Errorf("failed to convert recordSize: %v", err)
	}

	keysNumber, err := strconv.Atoi(record[2])
	if err != nil {
		return nil, fmt.Errorf("failed to convert keysNumber: %v", err)
	}

	definitions, err := strconv.Atoi(record[3])
	if err != nil {
		return nil, fmt.Errorf("failed to convert definitions: %v", err)
	}
	//create the new table with the parsed values
	table := NewTable(binaryPath, kbName, tableName, hashSize, recordSize, keysNumber, fields, definitions, cached, nil)
	return table, nil
}

// GetTableName returns the table name in the database
func (t *TableDefinition) GetTableName() string {
	return t.KbName
}

// GetHashSize returns the configured hash size
func (t *TableDefinition) GetHashSize() int {
	return t.HashSize
}

// GetKeysNumber returns the number of keys defined for the table
func (t *TableDefinition) GetKeysNumber() int {
	return t.KeysNumber
}

// GetFields returns the list of fields defined in the table
func (t *TableDefinition) GetFields() map[string]int {
	return t.Fields
}

// ValidateField checks if a field exists in the table definition
func (t *TableDefinition) ValidateField(field string) (int, error) {
	if pos, exist := t.Fields[field]; exist {
		return pos, nil
	}
	return -1, fmt.Errorf("the field %s do not exist on table %s", field, t.TableName)
}

// ValidateKey verifies that the key has the correct size
func (t *TableDefinition) ValidateKey(key []byte) error {
	if len(key) != t.HashSize {
		return fmt.Errorf("invalid key size: expected %d bytes, got %d bytes",
			t.HashSize, len(key))
	}
	return nil
}

// RecordHandler is a function type that processes records
type RecordHandler_T func(table *TableDefinition, key, record []byte) bool

// FetchRecordset reads and processes records from a sector
func (table *TableDefinition) FetchRecordset(s *Sector, key []byte, skipSubkey bool, dataChan chan []string, closeChanAtEnd bool) (int, error) {

	//Handler to process ldb records
	handler := func(table *TableDefinition, key, record []byte) bool {
		data := []string{fmt.Sprintf("%02x", key)}
		for i := 0; i < table.KeysNumber-1; i++ {
			subkey := record[i*table.HashSize : (i+1)*table.HashSize]
			subkeyHex := fmt.Sprintf("%02x", subkey)
			data = append(data, subkeyHex)
		}

		if len(record) > table.recordSize*(table.KeysNumber-1) {
			msg, err := DecodeTable(record, table, key)
			if err != nil {
				fmt.Println(err)
				return false
			}
			data = append(data, strings.Split(string(msg), ",")...)
		}
		dataChan <- data
		return false
	}

	if key == nil {
		return 0, fmt.Errorf("invalid key")
	}
	//the fetchRecordset can operate with a ram loaded sector or over a file. The first case is used by the dump operation, and the second one for the standar query.
	sector := Sector{ID: key[0], Data: nil, File: nil}
	var sectorP *Sector
	if s == nil {
		sectorP = &sector
	} else {
		sectorP = s
	}
	//calculate the subKeyLen
	subkeyLen := table.HashSize - LDB_KEY_LN
	records := 0
	done := false

	// Process nodes until we reach the end or handler signals completion
	var nextPtr uint64
	for !done {
		// Read node
		next, nodeData, err := ReadNode(sectorP, table, nextPtr, key, 0)
		if err != nil {
			return records, fmt.Errorf("error reading table %s/%s - sector %02x: %v",
				table.KbName, table.TableName, sector.ID, err)
		}

		if len(nodeData) == 0 && next == 0 {
			break // reached end of list
		}

		// Handle fixed-length records
		if table.recordSize > 0 {
			done = handler(table, key, nodeData)
			records++
			nextPtr = next
			continue
		}

		// Handle variable-length records
		if !validateNode(nodeData, subkeyLen) {
			nextPtr = next
			continue
		}

		// Process node data
		nodePtr := 0
		for nodePtr < len(nodeData) && !done {
			// Extract subkey
			if nodePtr+int(subkeyLen) > len(nodeData) {
				break
			}
			subkey := nodeData[nodePtr : nodePtr+int(subkeyLen)]
			nodePtr += int(subkeyLen)

			// Get dataset size
			if nodePtr+2 > len(nodeData) {
				break
			}
			datasetSize := ReadUint16(nodeData[nodePtr:])
			nodePtr += 2

			// Check if subkey matches
			keyMatched := true
			if !skipSubkey && subkeyLen > 0 {
				keyMatched = compareBytes(subkey, key[LDB_KEY_LN:LDB_KEY_LN+subkeyLen])
			}

			if keyMatched {
				// Process records in dataset
				datasetPtr := 0
				for datasetPtr < int(datasetSize) {
					// Get record size
					if nodePtr+datasetPtr+2 > len(nodeData) {
						break
					}
					recordSize := ReadUint16(nodeData[nodePtr+datasetPtr:])
					datasetPtr += 2

					// Process record if it's not too large
					if recordSize+32 < LDB_MAX_REC_LN {
						if nodePtr+datasetPtr+int(recordSize) > len(nodeData) {
							break
						}
						record := nodeData[nodePtr+datasetPtr : nodePtr+datasetPtr+int(recordSize)]
						fullKey := key
						if len(fullKey) < table.HashSize {
							fullKey = append(fullKey, subkey...)
						}

						done = handler(table, fullKey, record)
						records++
					}
					datasetPtr += int(recordSize)
				}
			}
			nodePtr += int(datasetSize)
		}

		nextPtr = next
		if nextPtr == 0 {
			done = true
		}
	}

	// Close file if it was opened
	if sector.File != nil {
		sector.File.Close()
		sector.File = nil
	}

	if closeChanAtEnd {
		close(dataChan)
	}

	return records, nil
}

// compareBytes compares two byte slices
func compareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Dump a complete sector. The sector is loadead in ram. If no sector is specified, all the table is dumped.
func (t *TableDefinition) Dump(startingSector int, endingSector int, limit int, dataChan chan []string) (int, error) {
	from := 0
	to := 255
	count := 0
	if startingSector > 255 || endingSector > 255 {
		return -1, fmt.Errorf("invalid starting / ending sector")
	}

	defer close(dataChan)
	if startingSector >= 0 {
		from = startingSector
	}

	if endingSector >= 0 {
		to = endingSector
	}

	for k0 := from; k0 <= to; k0++ {
		sector := ldbLoadSector(t, []byte{byte(k0)})
		for k1 := 0; k1 < 256; k1++ {
			for k2 := 0; k2 < 256; k2++ {
				for k3 := 0; k3 < 256; k3++ {
					key := []byte{byte(k0), byte(k1), byte(k2), byte(k3)}
					recordsNumber, err := t.FetchRecordset(&sector, key, true, dataChan, false)
					if err != nil {
						return count, err
					}
					count += int(recordsNumber)
					if limit > 0 && count > limit {
						return count, nil
					}
				}
			}
		}
	}
	return count, nil
}

func (t *TableDefinition) QueryKey(keyHex string) ([][]string, error) {

	outputChan := make(chan []string, 1024)
	queryError := false
	var err error
	go func() {
		_, err = t.Query(keyHex, outputChan, true)
		if err != nil {
			queryError = true
		}
	}()
	result := make([][]string, 0)
	for r := range outputChan {
		result = append(result, r)
	}
	if queryError {
		return result, fmt.Errorf("fetchRecordset has vailed with error %v", err)
	}
	return result, nil
}

func (t *TableDefinition) DumpParallel(startingSector int, endingSector int, limit int, dataChan chan []string) error {
	from := 0
	to := 255

	if startingSector > 255 || endingSector > 255 {
		return fmt.Errorf("invalid starting / ending sector")
	}
	if startingSector >= 0 {
		from = startingSector
	}
	if endingSector >= 0 {
		to = endingSector
	}

	threadsNumber := endingSector - startingSector
	if threadsNumber <= 0 {
		threadsNumber = 1
	}
	if threadsNumber > 32 {
		threadsNumber = 32
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, threadsNumber)
	errChan := make(chan error, 1)

	for k0 := from; k0 <= to; k0++ {
		semaphore <- struct{}{}
		wg.Add(1)
		k0Local := k0

		go func() {
			defer func() {
				<-semaphore
				wg.Done()
			}()

			sector := ldbLoadSector(t, []byte{byte(k0Local)})

			for k1 := 0; k1 < 256; k1++ {
				for k2 := 0; k2 < 256; k2++ {
					for k3 := 0; k3 < 256; k3++ {
						key := []byte{byte(k0Local), byte(k1), byte(k2), byte(k3)}
						if _, err := t.FetchRecordset(&sector, key, true, dataChan, false); err != nil {
							select {
							case errChan <- err:
							default:
							}
							return
						}
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(dataChan)
	}()

	select {
	case err := <-errChan:
		return err
	case <-dataChan:
		return nil
	}
}

func (t *TableDefinition) addData2Cache(data []string) error {
	if !t.cached {
		return nil
	}

	if len(data) == 0 {
		return fmt.Errorf("invalid data: empty slice received")
	}

	if len(data[0]) < 2 {
		return fmt.Errorf("invalid data: first element '%s' is too short for sector extraction", data[0])
	}

	sector, err := strconv.ParseInt(data[0][:2], 16, 64)
	if err != nil {
		return fmt.Errorf("invalid sector format in '%s': %w", data[0][:2], err)
	}

	if t.cache[sector] == nil {
		t.cache[sector] = make(map[string][][]string)
	}

	newdata := true
	/*if records, exists := t.cache[sector][data[0]]; exists {

		for _, record := range records {
			if len(record) != len(data)-1 {
				continue
			}

			matches := true
			for i, field := range record {
				if field != data[i+1] {
					matches = false
					break
				}
			}
			if matches {
				newdata = false
				break
			}
		}
	}*/

	if newdata {
		dataCopy := make([]string, len(data)-1)
		copy(dataCopy, data[1:])

		t.cache[sector][data[0]] = append(t.cache[sector][data[0]], dataCopy)
	}

	return nil
}

func (t *TableDefinition) GetDataFromCache(sectorID int, keyHex string, dataChan chan<- []string) (int, error) {
	if !t.cached {
		return 0, fmt.Errorf("cache is not enabled for this table")
	}
	if dataChan == nil {
		return 0, fmt.Errorf("invalid channel: nil channel received")
	}
	if sectorID < 0 || sectorID >= len(t.cache) {
		return 0, fmt.Errorf("invalid sector ID: %d is out of range [0,%d]", sectorID, len(t.cache)-1)
	}
	if t.cache[sectorID] == nil {
		t.cache[sectorID] = make(map[string][][]string)
		return 0, nil
	}

	// Si keyHex está vacío, enviar todos los registros del sector
	if keyHex == "" {
		totalRecords := 0
		for key, records := range t.cache[sectorID] {
			for _, record := range records {
				data := make([]string, len(record)+1)
				data[0] = key
				copy(data[1:], record)
				dataChan <- data
				totalRecords++
			}
		}
		return totalRecords, nil
	}

	records := t.cache[sectorID][keyHex]
	if len(records) == 0 {
		return 0, nil
	}

	for _, record := range records {
		data := make([]string, len(record)+1)
		data[0] = keyHex
		copy(data[1:], record)
		dataChan <- data
	}
	return len(records), nil
}
