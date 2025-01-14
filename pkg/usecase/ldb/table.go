package ldb

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TableDefinition defines the base structure of a table
type TableDefinition struct {
	KbName      string
	TableName   string
	HashSize    int            // Hash size in bytes
	KeysNumber  int            // Number of keys (1 primary + N secondary)
	Fields      map[string]int // Table fields
	Definitions int
	recordSize  int //0 means variable size
	virtual     bool
	ts_ln       uint32
	path        string
}

// NewTable creates a new table definition with default values
func NewTable(tablePath string, kbName string, tableName string, hashSize int, recordSize int, keysNumber int, fields []string, definitions int, virtual bool, data [][]string) *TableDefinition {

	if kbName == "" {
		kbName = "oss"
	}

	if tablePath == "" {
		tablePath = LdbDefaultPath + "/" + kbName + "/" + tableName
	}
	cols := make(map[string]int)
	for i, f := range fields {
		cols[f] = i
	}
	return &TableDefinition{
		KbName:      kbName,
		TableName:   tableName,
		HashSize:    hashSize,   // Default 16 bytes
		KeysNumber:  keysNumber, // Default 1 key
		Fields:      cols,       // Default data field only
		Definitions: definitions,
		recordSize:  recordSize,
		virtual:     virtual,
		ts_ln:       2,
		path:        tablePath,
	}
}

// NewTableFromCfg creates a new table from the configuration files
func NewTableFromCfg(ldbPath string, kbName string, tableName string, fields []string) (*TableDefinition, error) {
	// Set the default path.
	if ldbPath == "" {
		ldbPath = LdbDefaultPath
	}
	//Set the default kbname
	if kbName == "" {
		kbName = LdbDeFaultKbName
	}

	//Build the path to the configuration file
	configPath := filepath.Join(ldbPath, kbName, tableName+".cfg")

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
	tablePath := filepath.Join(ldbPath, kbName, tableName)
	//create the new table with the parsed values
	table := NewTable(tablePath, kbName, tableName, hashSize, recordSize, keysNumber, fields, definitions, false, nil)
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
	sectorId := -1
	count := 0
	defer close(dataChan)
	if startingSector >= 0 {
		sectorId = startingSector
	}
	for k0 := sectorId; k0 < 256; k0++ {
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
		//dump only the specified sector
		if (sectorId >= 0 && endingSector == 0) || (endingSector > 0 && k0 > endingSector) {
			return count, nil
		}
	}
	return count, nil
}

func (t *TableDefinition) QueryKey(keyHex string) ([][]string, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("error decoding key %s - %v", keyHex, err)
	}

	outputChan := make(chan []string, 1024)
	_, err = t.FetchRecordset(nil, key, false, outputChan, true)
	if err != nil {
		return nil, fmt.Errorf("fetchRecordset has vailed with error %v", err)
	}

	result := make([][]string, 0)
	for r := range outputChan {
		result = append(result, r)
	}
	return result, nil
}
