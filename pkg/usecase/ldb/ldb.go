package ldb

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LDB contants definition
const (
	LdbTableDefinitionStandard   = 0
	LdbTableDefinitionEncrypted  = 1
	LdbTableDefinitionMz         = 2
	LdbTableDefinitionCompressed = 4
	CryptoStreamNonceBytes       = 24
	LDB_PTR_LN                   = 5 // uint40 operations
	LDB_KEY_LN                   = 4
	LDB_MAX_REC_LN               = 65535
	LdbDefaultPath               = "/var/lib/ldb/"
	LdbDeFaultKbName             = "oss"
)

// GetMapPointerPos returns the map position for the given record ID
// using only the last 3 bytes of the key, since the sector name contains the first byte
func getMapPointerPos(key []byte) uint64 {
	var out uint64
	// Create a byte slice to store the result
	result := make([]byte, 8)
	// Assign less significant bytes (inverting for easy debugging)
	result[0] = key[3]
	result[1] = key[2]
	result[2] = key[1]
	// Convert to uint64
	out = binary.LittleEndian.Uint64(result)
	return out * LDB_PTR_LN
}

// Cast a uint40 as uint64
func readUint40(buffer []byte) uint64 {
	var result uint64
	for i := 0; i < LDB_PTR_LN; i++ {
		result |= uint64(buffer[i]) << (8 * uint(i))
	}
	return result
}

// GetListPointer gets the list pointer from the sector file
func getListPointer(file *os.File, key []byte) (uint64, error) {
	/*WARNING!!!: cast from uint64 to int64, but ldb uses a uint40 index, hight risk of overflow */
	_, err := file.Seek(int64(getMapPointerPos(key)), io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("seek error: %v", err)
	}
	buffer := make([]byte, LDB_PTR_LN)
	n, err := file.Read(buffer)
	if err != nil || n != LDB_PTR_LN {
		return 0, fmt.Errorf("cannot read LDB sector: %v", err)
	}
	return readUint40(buffer), nil
}

// ReadUint16 reads a 2-byte unsigned integer from a byte slice
func ReadUint16(data []byte) uint16 {
	return binary.LittleEndian.Uint16(data)
}

// ReadUint32 reads a 4-byte unsigned integer from a byte slice
func ReadUint32(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data)
}

// Sector represents an LDB sector in memory or on disk
type Sector struct {
	ID      byte
	Data    []byte
	Size    int64
	File    *os.File
	Failure bool
}

// ReadNode reads a node from the sector
func ReadNode(sector *Sector, table *TableDefinition, ptr uint64, key []byte, maxNodeSize int) (nextNode uint64, data []byte, err error) {
	var buffer []byte
	//Check if the sector is already loaded in memory
	if sector.Data != nil {
		if ptr == 0 {
			// Get the list location from the map
			mapPos := getMapPointerPos(key)
			if mapPos >= uint64(len(sector.Data)-LDB_PTR_LN) {
				return 0, nil, fmt.Errorf("map position out of range")
			}
			ptr = readUint40(sector.Data[mapPos : mapPos+LDB_PTR_LN])
			if ptr == 0 {
				return 0, nil, nil
			}
			ptr += LDB_PTR_LN
		}

		if uint64(len(sector.Data)) <= ptr {
			sector.Failure = true
			return 0, nil, fmt.Errorf("node pointer out of range: %d / %d", ptr, len(sector.Data))
		}
		buffer = sector.Data[ptr:]
	} else {
		if sector.File == nil {
			var err error
			sector.File, err = openLDBFile(table, key)
			if err != nil {
				return 0, nil, err
			}
		}

		if ptr == 0 {
			var err error
			ptr, err = getListPointer(sector.File, key)
			if err != nil {
				return 0, nil, err
			}
			if ptr == 0 {
				return 0, nil, nil
			}
			ptr += LDB_PTR_LN
		}
		/*WARNING!!!: cast from uint64 to int64, but ldb uses a uint40 index, hight risk of overflow */
		_, err := sector.File.Seek(int64(ptr), io.SeekStart)
		if err != nil {
			return 0, nil, err
		}

		buffer = make([]byte, LDB_PTR_LN+table.ts_ln+LDB_KEY_LN)
		n, err := sector.File.Read(buffer[:LDB_PTR_LN+table.ts_ln])
		if err != nil || n != int(LDB_PTR_LN+table.ts_ln) {
			return 0, nil, fmt.Errorf("cannot read LDB node: %v", err)
		}
	}

	// Read next node pointer
	nextNode = readUint40(buffer[:LDB_PTR_LN])

	// Read node size
	var nodeSize uint32
	if table.ts_ln == 2 {
		nodeSize = uint32(ReadUint16(buffer[LDB_PTR_LN:]))
	} else {
		nodeSize = ReadUint32(buffer[LDB_PTR_LN:])
	}

	actualSize := nodeSize
	if table.recordSize > 0 {
		actualSize *= uint32(table.recordSize)
	}

	// Check max node size
	if maxNodeSize > 0 && int(actualSize) > maxNodeSize {
		actualSize = 0
	}

	// Handle fixed-length records size limit
	if table.recordSize > 0 && actualSize > 64800 {
		actualSize = 64800
	}

	if actualSize > 0 {
		data = make([]byte, actualSize)
		if sector.Data != nil {
			if ptr+uint64(LDB_PTR_LN+table.ts_ln+actualSize) <= uint64(len(sector.Data)) {
				copy(data, sector.Data[ptr+uint64(LDB_PTR_LN+table.ts_ln):])
			} else {
				return 0, nil, fmt.Errorf("sector %02x node size overflow", sector.ID)
			}
		} else {
			n, err := sector.File.Read(data)
			if err != nil || n != int(actualSize) {
				return 0, nil, fmt.Errorf("cannot read entire LDB node: %v", err)
			}
		}
	}

	return nextNode, data, nil
}

// Constants (assuming these were defined elsewhere in the C code)
const LDB_MAX_PATH = 256 // Adjust this value according to your needs

// GetSectorPath returns the path for a sector file
func getSectorPath(table *TableDefinition, key []byte) (string, error) {
	// Create table path
	//tablePath := filepath.Join("/data/ldb/", table.KbName, table.TableName)

	// Check if directory exists
	if _, err := os.Stat(table.path); os.IsNotExist(err) {
		fmt.Printf("E063 Table %s does not exist\n", table.path)
		os.Exit(1)
	}

	// Create sector path
	sectorPath := filepath.Join(table.path, fmt.Sprintf("%02x.ldb", key[0]))

	// Check if file exists
	_, err := os.Stat(sectorPath)

	return sectorPath, err
}

// OpenLDBFile opens a sector file for reading
func openLDBFile(table *TableDefinition, key []byte) (*os.File, error) {
	if key == nil {
		return nil, fmt.Errorf("key cannot be nil")
	}

	// Get the sector path
	sectorPath, err := getSectorPath(table, key)
	if err != nil {
		return nil, err
	}

	// Open data sector in read-only mode
	file, err := os.OpenFile(sectorPath, os.O_RDONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open LDB sector %s: %v", sectorPath, err)
	}

	return file, err
}

// ValidateNode checks if a node's structure is valid. Returns false if it isn't
func validateNode(node []byte, subkeyLen int) bool {
	nodeSize := uint32(len(node))
	// Make sure we have enough bytes in the node
	if nodeSize < uint32(subkeyLen+2) {
		return false
	}

	// Extract datasets from node
	var nodePtr uint32
	for nodePtr < nodeSize {
		// Skip subkey
		nodePtr += uint32(subkeyLen)

		// Check if we have enough bytes to read dataset size.
		//The dataset size is saved in a uint16 (two byes).
		if nodePtr+2 > nodeSize {
			return false
		}

		// Get dataset size
		datasetSize := uint32(ReadUint16(node[nodePtr:]))
		nodePtr += 2

		// Is the reported dataset_size greater than the remaining node?
		if nodePtr+datasetSize > nodeSize {
			return false
		}

		// Extract records from dataset
		if subkeyLen > 0 {
			var datasetPtr uint32
			for datasetPtr < datasetSize {
				// Check if we have enough bytes to read record size. Again the recordset size is an uint16 integer.
				if nodePtr+datasetPtr+2 > nodeSize {
					return false
				}

				// Get record size
				recordSize := uint32(ReadUint16(node[nodePtr+datasetPtr:]))
				datasetPtr += 2

				// Is the reported record_size greater than the remaining dataset?
				if nodePtr+datasetPtr+recordSize > nodeSize {
					return false
				}

				// Move pointer to end of record
				datasetPtr += recordSize

				// If we passed the dataset_size, the node is bad
				if datasetPtr > datasetSize {
					return false
				}
			}
		}

		// Move pointer to end of dataset
		nodePtr += datasetSize
	}
	return true
}

// ldbLoadSector loads a LDB sector file into memory (Sector structure)
func ldbLoadSector(table *TableDefinition, key []byte) Sector {
	//Creates a new sector with the default values.
	sector := Sector{
		ID:      key[0],
		Data:    nil,
		Size:    0,
		File:    nil,
		Failure: false,
	}

	//Open the sector file
	ldbSector, err := openLDBFile(table, key)
	if err != nil {
		sector.Failure = true
		return sector
	}
	defer ldbSector.Close()

	//Get the sector size
	fileInfo, err := ldbSector.Stat()
	if err != nil {
		sector.Failure = true
		return sector
	}
	size := fileInfo.Size()

	// Allocate memory for reading the sector data.
	data := make([]byte, size)

	// Read the sector and save in memory
	_, err = ldbSector.Read(data)
	if err != nil {
		sector.Failure = true
		return sector
	}

	// Update the sector structure.
	sector.Data = data
	sector.Size = size

	return sector
}
