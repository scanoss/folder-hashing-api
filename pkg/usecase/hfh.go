package usecase

import (
	"fmt"
	"log"
	"math/bits"
	"strconv"
	"sync"

	pb "github.com/scanoss/papi/api/scanningv2"
	ldb "scanoss.com/hfh-api/pkg/usecase/ldb"
)

type HFHscan struct {
	hfhTable    *ldb.TableDefinition
	hfhSecTable *ldb.TableDefinition
	urlTable    *ldb.TableDefinition
	threshold   int
	bestMatch   bool
	dMin        int
	sectorTol   int
	resultsMap  map[string]HFHscanResult
}

type HFHscanResultItem struct {
	Purl       string
	Versions   []string
	Confidence float32
}

type HFHscanResult struct {
	components  []HFHscanResultItem
	stage       int
	probability float32
}

func NewHFFHScan(threshold int, bestMatch bool, ldbPath string, kbName string) (*HFHscan, error) {
	urlTable, err := ldb.NewTableFromCfg(ldbPath, kbName, "url", []string{"key", "component", "vendor", "version", "date", "license", "purl", "url", "a", "b", "c", "d", "e"})
	if err != nil {
		return nil, fmt.Errorf("error creating urlTable: %v", err)
	}

	hfhTable, err := ldb.NewTableFromCfg(ldbPath, kbName, "hfh", []string{"fileNames", "fileContents", "url"})
	if err != nil {
		return nil, fmt.Errorf("error creating HFHtable: %v", err)
	}

	hfhSecTable, err := ldb.NewTableFromCfg(ldbPath, kbName, "hfhSec", []string{"partialFileContents", "fileNames"})
	if err != nil {
		return nil, fmt.Errorf("error creating HFHsecTable: %v", err)
	}

	return &HFHscan{hfhTable: hfhTable,
		urlTable:    urlTable,
		hfhSecTable: hfhSecTable,
		bestMatch:   bestMatch,
		threshold:   threshold,
		dMin:        30,
		sectorTol:   8,
		resultsMap:  make(map[string]HFHscanResult)}, nil
}

func (s *HFHscan) Scan(projectTree *pb.HFHRequest_Children) error {
	return nil
}

func hammingDistance(x, y uint64) int {
	xor := x ^ y
	return bits.OnesCount64(xor)
}

func scanHash(table *ldb.TableDefinition, hashHex string, sectorTol int, minDistance int) ([]uint64, int, [][]string, error) {

	hash, err := strconv.ParseUint(hashHex, 16, 64)
	if err != nil {
		return nil, -1, nil, fmt.Errorf("error decoding key %s - %v", hashHex, err)
	}

	if hash == 0xffffffffffffffff {
		return nil, -1, nil, fmt.Errorf("scan aborted, the path is empty")
	}
	//Try an exact match with the filenames key

	records, err := table.QueryKey(hashHex)
	if err != nil {
		fmt.Errorf("error quering key %s - %v", hashHex, err)
	}
	if len(records) > 0 {
		var closestMatches []uint64
		var contents [][]string
		for _, r := range records {
			key, err := strconv.ParseUint(r[0], 16, 64)
			if err != nil || key == 0xffffffffffffffff {
				continue
			}
			closestMatches = append(closestMatches, key)
			contents = append(contents, r[1:])
		}
		return closestMatches, 0, contents, nil
	}

	//Look for approximity matches

	//Define the sectors to be dumped
	head := (hash & 0xFF00000000000000) >> 56
	start := int(head) - sectorTol
	if start < 0 {
		start = 0
	}
	end := int(head) + sectorTol
	if end > 255 {
		end = 255
	}
	//process each sector, run this process in a new thread
	var wg sync.WaitGroup
	wg.Add(1)
	outputChan := make(chan []string, 1024*sectorTol)

	// Exec dump function in its own thread
	go func() {
		defer wg.Done()
		var err error
		_, err = table.Dump(start, end, -1, outputChan)
		if err != nil {
			log.Printf("Unexpected error during dump: %v", err)
		}
	}()

	// Find the closest matches for the hash
	var closestMatches []uint64
	var contents [][]string

	for record := range outputChan {
		if len(record) > 0 {
			key, err := strconv.ParseUint(record[0], 16, 64)
			if err != nil || key == 0xffffffffffffffff {
				continue
			}
			distance := hammingDistance(hash, key)
			if distance < minDistance {
				closestMatches = []uint64{key}
				minDistance = distance
				contents = [][]string{record[1:]}
			} else if distance == minDistance {
				closestMatches = append(closestMatches, key)
				contents = append(contents, record[1:])
			}
		}
	}

	return closestMatches, minDistance, contents, nil
}

// retrieves purl information from the url table
func getComponents(table *ldb.TableDefinition, urls []string) []HFHscanResultItem {
	uniquePurlVersions := make(map[string]bool)
	purlVersionMap := make(map[string][]string)

	for _, urlKey := range urls {
		urls, err := table.QueryKey(urlKey)
		if err != nil {
			log.Println(err)
			continue
		}
		for _, url := range urls {
			//extract the purl
			purl := url[6]
			//extract the version
			version := url[3]
			//use purl+version to track unique components
			if _, exist := uniquePurlVersions[purl+version]; exist {
				continue
			}
			uniquePurlVersions[purl+version] = true
			purlVersionMap[purl] = append(purlVersionMap[purl], version)
		}
	}
	components := make([]HFHscanResultItem, len(purlVersionMap))
	i := 0
	for purl := range purlVersionMap {
		components[i].Purl = purl
		components[i].Versions = purlVersionMap[purl]
		i++
	}
	return components
}

func (s *HFHscan) scanFirstStage(fileNamesSimhash string, fileContentsSimhash string) (*HFHscanResult, error) {

	//get the matching candidates from the HFH table based on the file names hash
	result, distance, content, err := scanHash(s.hfhTable, fileNamesSimhash, s.sectorTol, s.dMin)
	if err != nil {
		return nil, err
	}

	if len(result) > 0 {
		log.Printf("File Names match: %x with distance %d", result[0], distance)
	}
	//select the best based on the file contents hash
	minFilesContentdistance := s.dMin * 3 / 4
	if distance > s.dMin {
		return nil, fmt.Errorf("no results")
	}

	var lastKey uint64 = 0
	var bestMatches []uint64
	var bestUrlsKey []string

	for i, r := range result {
		if r == lastKey {
			log.Printf("Skiping repeated key %x\n", r)
			continue
		}
		key, err := strconv.ParseUint(content[i][0], 16, 64)
		if err != nil {
			log.Println(err)
			continue
		}
		lastKey = key
		contentsHash, err := strconv.ParseUint(fileContentsSimhash, 16, 64)
		if err != nil {
			log.Println(err)
			continue
		}

		filesContentdistance := hammingDistance(contentsHash, key)
		if filesContentdistance < minFilesContentdistance {
			bestMatches = []uint64{key}
			minFilesContentdistance = filesContentdistance
			bestUrlsKey = []string{content[i][1]}
		} else if filesContentdistance == minFilesContentdistance {
			bestMatches = append(bestMatches, key)
			bestUrlsKey = append(bestUrlsKey, content[i][1])
		}
	}

	if len(bestUrlsKey) > 0 {

		log.Printf("file content %s distance: %d - URL: %s", fileContentsSimhash, minFilesContentdistance, bestUrlsKey)
		probability := (1 - float32(minFilesContentdistance)/float32(s.dMin)) * 100
		st := 1
		if probability < 0 {
			probability = (1 - float32(distance)/float32(s.dMin)) * 100
			st = 0
		}

		fistStageComponents := getComponents(s.urlTable, bestUrlsKey)
		return &HFHscanResult{components: fistStageComponents, probability: probability, stage: st}, nil
	}

	probability := (1 - float32(distance)/float32(s.dMin)) * 100
	urls := make([]string, len(content))
	for i, c := range content {
		urls[i] = c[1]
	}
	fistStageComponents := getComponents(s.urlTable, urls)

	return &HFHscanResult{components: fistStageComponents, probability: probability, stage: 0}, nil
}

func (s *HFHscan) scanTreeFirstStage(node *pb.HFHRequest_Children) error {

	result, err := s.scanFirstStage(node.SimHashNames, node.SimHashContent)
	if err != nil {
		return err
	}
	if result.probability >= float32(s.threshold) {
		if result.stage == 0 {
			result.probability /= 2
		}
		s.resultsMap[node.PathId] = *result
	}

	if result.probability < float32(s.threshold) || result.stage == 0 {
		for _, child := range node.Children {
			s.scanTreeFirstStage(child)
		}
	}
	return nil
}

func (s *HFHscan) scanSecondStage(fileContentsSimhash string) (*HFHscanResult, error) {

	hash, err := strconv.ParseUint(fileContentsSimhash, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("error decoding key %s - %v", fileContentsSimhash, err)
	}
	head := headCalc(hash)
	//Overwrite the MS byte with the head to keep the hash size total
	key := (hash & 0x00FFFFFFFFFFFFFF) | (uint64(head) << 56)
	keyHex := fmt.Sprintf("%02x", key)
	log.Printf("filecontents query hash %s\n", keyHex)
	match, distance, content, err := scanHash(s.hfhSecTable, keyHex, s.sectorTol/4, s.dMin/3)
	if err != nil {
		return nil, err
	}

	log.Printf("secHash: %x - matched: %x- distance %d\n", key, match, distance)
	var urlKeys []string
	for _, c := range content {
		//go to the main hfh table to look for the url hash
		key := c[0]
		records, err := s.hfhTable.QueryKey(key)
		if err != nil {
			fmt.Println(err)
			continue
		}
		for _, r := range records {
			urlKeys = append(urlKeys, r[2])
		}
	}

	probability := (1 - float32(distance)/float32(s.dMin)) * 100
	components := getComponents(s.urlTable, urlKeys)

	return &HFHscanResult{components: components, probability: probability, stage: 2}, nil
}

func (s *HFHscan) scanTreeSecondStage(node *pb.HFHRequest_Children) error {

	if s.resultsMap[node.PathId].stage > 0 && s.resultsMap[node.PathId].probability > float32(s.threshold) {
		return nil
	}

	result, err := s.scanSecondStage(node.SimHashContent)
	if err != nil {
		return err
	}
	if result.probability >= float32(s.threshold) && result.components != nil {
		s.resultsMap[node.PathId] = *result
	}

	if s.resultsMap[node.PathId].probability < float32(s.threshold) || s.resultsMap[node.PathId].stage == 0 {
		for _, child := range node.Children {
			s.scanTreeSecondStage(child)
		}
	}

	return nil
}
