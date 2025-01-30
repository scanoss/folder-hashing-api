package ldb

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

func TestLDBcreateTable(t *testing.T) {
	testTable, err := NewTableFromCfg("./test", "test_kb", "hfh", []string{"fileNames", "fileContents", "url"}, false)
	if err != nil {
		t.Fatalf("an error '%s' was not expected reading the table config", err)
		return
	}

	if testTable.KeysNumber != 3 || testTable.HashSize != 8 {
		t.Errorf("table config file parsing error, the table configuration doesn't match")
		return
	}
}

func TestLDBdecode(t *testing.T) {

	/* Decode this record:
	00075de93df18ea6,271a83cbd2ce447978f5b2613706c2928758197999218c06921d82b96c09e04bd8734a0f3b56785a55fd79b53d10b8b369ac01a45083d17b3bf867db84b2696ce6a401106929958886d61a588dec8a9a02eb218a4d2f223b8049fcefc7c0819a89ead32e2406fb94af25809fa0fbd3c1c3571b742324d76e22c80ea3ca7d9d85e0586ab3d6845cf3676199bac77f8a94460601e2f81f1ddce57e67e04d22b3
	*/
	keyStr := "00075de93df18ea6"
	dataStr := "271a83cbd2ce447978f5b2613706c2928758197999218c06921d82b96c09e04bd8734a0f3b56785a55fd79b53d10b8b369ac01a45083d17b3bf867db84b2696ce6a401106929958886d61a588dec8a9a02eb218a4d2f223b8049fcefc7c0819a89ead32e2406fb94af25809fa0fbd3c1c3571b742324d76e22c80ea3ca7d9d85e0586ab3d6845cf3676199bac77f8a94460601e2f81f1ddce57e67e04d22b3"
	resultStr := "Sebastian Gug,@decorfn/firebase,0.0.5,2019-07-08,,pkg:npm/%40decorfn/firebase,https://registry.npmjs.org/@decorfn/firebase/-/firebase-0.0.5.tgz,19,19,11,0,6052"
	key, err := hex.DecodeString(keyStr)

	if err != nil {
		t.Errorf("Error decoding key %s - %v\n", keyStr, err)
		return
	}

	data, err := hex.DecodeString(dataStr)
	if err != nil {
		t.Errorf("Error decoding data %s - %v\n", dataStr, err)
		return
	}

	urlTable, err := NewTableFromCfg("./test", "test_kb", "url", []string{"key", "component", "vendor", "version", "date", "license", "purl", "url", "a", "b", "c", "d", "e"}, false)
	if err != nil {
		t.Errorf("Error creating urlTable: %v", err)
		return
	}

	decoded, err := DecodeTable(data, urlTable, key)
	if err != nil {
		t.Errorf("Error decoding: %v", err)
		return
	}
	if string(decoded) != resultStr {
		t.Errorf("decoded string doesn't match with the expected result: %s\n", string(decoded))
	}
}

func TestLDBfetchRecordset(t *testing.T) {

	keyStr := "00075de93df18ea6"
	resultStr := "00075de93df18ea6,Sebastian Gug,@decorfn/firebase,0.0.5,2019-07-08,,pkg:npm/%40decorfn/firebase,https://registry.npmjs.org/@decorfn/firebase/-/firebase-0.0.5.tgz,19,19,11,0,6052"
	result := strings.Split(resultStr, ",")
	key, err := hex.DecodeString(keyStr)

	if err != nil {
		t.Errorf("Error decoding key %s - %v\n", keyStr, err)
		return
	}

	urlTable, err := NewTableFromCfg("./test", "test_kb", "url", []string{"key", "component", "vendor", "version", "date", "license", "purl", "url", "a", "b", "c", "d", "e"}, false)
	if err != nil {
		t.Errorf("Error creating urlTable: %v", err)
		return
	}
	outputChan := make(chan []string, 10)
	records, err := urlTable.FetchRecordset(nil, key, false, outputChan, true)
	if err != nil {
		t.Errorf("fetchRecordset has vailed with error %v", err)
		return
	}
	receivedRecords := 0
	for r := range outputChan {
		t.Log(r)
		for i, field := range r {
			if field != result[i] {
				t.Errorf("Result field %d doesn't match: %s / %s\n", i, field, result[i])
				return
			}
		}
		receivedRecords++
	}

	if receivedRecords != int(records) {
		t.Errorf("Received records and fetchRecorset returned value do not match: %d / %d \n", receivedRecords, records)
	}

}

func TestLDBdump(t *testing.T) {
	hfhTable, err := NewTableFromCfg("./test", "test_kb", "hfh", []string{"fileNames", "fileContents", "url"}, false)
	if err != nil {
		t.Errorf("Error creating hfhTable: %v", err)
		return
	}
	outputChan := make(chan []string, 1024)
	var wg sync.WaitGroup
	wg.Add(1)

	var recordsNumber int

	// Exec dump function in its own thread
	go func() {
		defer wg.Done()
		var err error
		recordsNumber, err = hfhTable.Dump(0x80, 0, 1023, outputChan)
		if err != nil {
			t.Errorf("Unexpected error during dump: %v", err)
			return
		}
	}()

	receivedRecords := 0
	var records [][]string
	for r := range outputChan {
		t.Log(r)
		records = append(records, r)
		receivedRecords++
	}

	if receivedRecords != recordsNumber {
		t.Errorf("Received records and fetchRecorset returned value do not match: %d / %d \n", receivedRecords, recordsNumber)
	}

	expectedRecord_0 := []string{"800000c9bc6bdfcd", "ffebfbdffeff5679", "b7df54874a01b857"}
	expectedRecord_last := []string{"8000a17597c207c2", "df9d796cca0c4e5e", "6cb1ac93ca8e7fae"}

	for i, field := range records[0] {
		if field == "" {
			break
		}
		if field != expectedRecord_0[i] {
			t.Errorf("Result field %d doesn't match: %s / %s\n", i, field, expectedRecord_0[i])
			return
		}
	}

	for i, field := range records[len(records)-1] {
		if field == "" {
			break
		}
		if field != expectedRecord_last[i] {
			t.Errorf("Result field %d doesn't match: %s / %s\n", i, field, expectedRecord_last[i])
			return
		}
	}
}
