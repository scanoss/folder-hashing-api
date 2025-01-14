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

	hfhSecTable, err := ldb.NewTableFromCfg(ldbPath, kbName, "url", []string{"partialFileContents", "fileNames"})
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

func scanHash(table *ldb.TableDefinition, hash uint64, sectorTol int, minDistance int) ([]uint64, int, [][]string) {

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

	return closestMatches, minDistance, contents
}

func (s *HFHscan) scanFirstStage(fileNamesSimhash uint64, fileContentsSimhash uint64) (int, float32, []HFHscanResultItem, error) {

	if fileNamesSimhash == 0xffffffffffffffff {
		return 0, 0, nil, fmt.Errorf("scan aborted, the path is empty")
	}

	result, distance, content := scanHash(s.hfhTable, fileNamesSimhash, s.sectorTol, s.dMin)
	if len(result) > 0 {
		log.Printf("File Names match: %x with distance %d", result[0], distance)
	}
	var fistStageResult []HFHscanResultItem
	var probability float32
	st := 0
	minFilesContentdistance := s.dMin * 3 / 4
	if distance < s.dMin {
		processedUrl := make(map[string]string)
		urlVersionMap := make(map[string]string)
		purlVersionMap := make(map[string][]string)
		uniqueVersions := make(map[string]bool)
		/*displays all versions for name hashes*/
		for _, c := range content {
			urlKey := c[1]
			if _, processed := processedUrl[urlKey]; processed {
				continue
			}

			urls, err := s.urlTable.QueryKey(urlKey)

			if err != nil {
				log.Println(err)
				continue
			}
			processedUrl[urlKey] = urls[0][6] //+ "@" + urls[0][3]
			version := urls[0][3]
			purl := urls[0][6]
			if _, exist := uniqueVersions[purl+version]; exist {
				continue
			}
			uniqueVersions[purl+version] = true
			urlVersionMap[urlKey] = version
			purlVersionMap[purl] = append(purlVersionMap[purl], version)
		}

		uniquePurls := make(map[string]bool)
		for _, component := range processedUrl {
			if _, processed := uniquePurls[component]; processed {
				continue
			}
			uniquePurls[component] = true
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
			filesContentdistance := hammingDistance(fileContentsSimhash, key)
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
			log.Printf("file content %x distance: %d - URL: %s", fileContentsSimhash, minFilesContentdistance, bestUrlsKey)
			probability = (1 - float32(minFilesContentdistance)/float32(s.dMin)) * 100
			st = 1
			if probability < 0 {
				probability = (1 - float32(distance)/float32(s.dMin)) * 100
				st = 0
			}
			uniquePurls := make(map[string]bool)
			var purlList []string
			for _, urlKey := range bestUrlsKey {
				if _, processed := uniquePurls[processedUrl[urlKey]]; processed {
					continue
				}
				uniquePurls[processedUrl[urlKey]] = true
				purlList = append(purlList, processedUrl[urlKey])
			}
			//fistStageResult = m.ScanResult_T{Purl: purlList, Prob: probability, Versions: bestVersions, Stage: st}
			fistStageResult = make([]HFHscanResultItem, len(purlList))
			for i, purl := range purlList {
				fistStageResult[i].Purl = purl
				fistStageResult[i].Versions = purlVersionMap[purl]
			}

		} else {
			probability = (1 - float32(distance)/float32(s.dMin)) * 100
			var purlList []string
			for purl, _ := range uniquePurls {
				purlList = append(purlList, purl)
			}
			//fistStageResult = m.ScanResult_T{Purl: purlList, Prob: probability, Versions: purlVersionMap, Stage: 0}
			fistStageResult = make([]HFHscanResultItem, len(purlList))
			for i, purl := range purlList {
				fistStageResult[i].Purl = purl
				fistStageResult[i].Versions = purlVersionMap[purl]

			}
		}

	}
	return st, probability, fistStageResult, nil
}

func (s *HFHscan) scanTreeFirstStage(node *pb.HFHRequest_Children) error {

	namesHash, err := strconv.ParseUint(node.SimHashNames, 16, 64)
	if err != nil {
		return fmt.Errorf("error decoding key %s - %v", node.SimHashNames, err)
	}

	contentsHash, err := strconv.ParseUint(node.SimHashContent, 16, 64)
	if err != nil {
		return fmt.Errorf("error decoding key %s - %v", node.SimHashContent, err)
	}

	if namesHash == 0xffffffffffffffff {
		return fmt.Errorf("scan aborted for %s, the path is empty", node.PathId)
	}

	stage, prob, components, err := s.scanFirstStage(namesHash, contentsHash)
	if err != nil {
		return err
	}
	var result HFHscanResult
	if prob >= float32(s.threshold) {
		if result.stage == 0 {
			result.probability /= 2
		}
		result.components = components
		result.stage = stage
		result.probability = prob
		s.resultsMap[node.PathId] = result
	}

	if result.probability < float32(s.threshold) || result.stage == 0 {
		for _, child := range node.Children {
			s.scanTreeFirstStage(child)
		}
	}
	return nil
}
