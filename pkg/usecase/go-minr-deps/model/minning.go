package model

import (
	"encoding/binary"
	"hash/crc64"
	"time"
)

type FileActions struct {
	CompletelyIgnore bool
	StoreInFile      bool
	StoreInPivot     bool
	StoreInMZ        bool
	StoreInExtraMZ   bool
	IsLicenseFile    bool
	//SkipSnippet       bool
}

type MiningMetadata struct {
	Vendor       string
	Component    string
	Version      string
	Release_date string
	License      string
	Purl         string //(see https://github.com/package-url/purl-spec)*/
	PurlHash     string //(see https://github.com/package-url/purl-spec)*/
	SrcHash      string
	Url          string
	//UrlHash       string
	TempLocation  string
	MineSource    byte
	Type          string
	Errors        bool
	Files         []string
	GenerateExtra bool
	IsRoot        bool
	FilesItems    []FileItem
	IgnoredFiles  int
	SourceFiles   int
	IndexedFiles  int
	Size          int64
}

type FileItem struct {
	Path   string
	KeyStr string
	Key    []byte
	//Content     []byte
	Actions     FileActions
	IsPreloaded bool
}
type MiningDownloadStatus struct {
	Total         int      `json:"total"`
	Downloaded    int      `json:"downloaded"`
	Processed     int      `json:"processed"`
	Failed        int      `json:"failed"`
	CurrDownloads []string `json:"current_download"`
	CurrProcess   []string `json:"current_processsing"`
}

type MiningMZStatus struct {
	Total     int     `json:"total"`
	Processed float32 `json:"processed"`
}

type MiningStatus struct {
	Stage    string                `json:"stage"`
	Download *MiningDownloadStatus `json:"download,omitempty"`

	Elapsed string          `json:"elapsed,omitempty"`
	Start   time.Time       `json:"-"`
	MZ      *MiningMZStatus `json:"wfp,omitempty"`
}

var CRCTable *crc64.Table
var hashLen uint8

func InitKeyHash() {
	CRCTable = crc64.MakeTable(crc64.ECMA)
	hashLen = 8
}

func GetHashBuff(buff []byte) []byte {
	hash := crc64.Checksum(buff, CRCTable)
	bytes := make([]byte, 8)
	// Convierte el uint64 a bytes en orden BigEndian
	binary.BigEndian.PutUint64(bytes, hash)
	return bytes

}

func GetHashUint(buff []byte) uint64 {
	hash := crc64.Checksum(buff, CRCTable)

	return hash

}
func GetHashLen() uint8 {
	return hashLen
}
