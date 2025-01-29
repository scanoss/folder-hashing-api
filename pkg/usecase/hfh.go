package usecase

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"sort"
	"strconv"
	"sync"

	"time"

	"go.uber.org/zap"
	myconfig "scanoss.com/hfh-api/pkg/config"
	"scanoss.com/hfh-api/pkg/dtos"
	ldb "scanoss.com/hfh-api/pkg/usecase/ldb"
)

type HFHscan struct {
	hfhTable    *ldb.TableDefinition
	hfhSecTable *ldb.TableDefinition
	urlTable    *ldb.TableDefinition
	thStage1    float32
	thStage2    float32
	thStage3    float32
	bestMatch   bool
	dMax        int
	sectorTol   int
	resultsMap  map[string]HFHscanResult
	s           *zap.SugaredLogger
}

type HFHscanResultItem struct {
	Purl       string    `json:"purl"`
	Versions   []string  `json:"versions"`
	Confidence float32   `json:"confidence"`
	Date       time.Time `json:"date"`
}

type HFHscanResult struct {
	Components  []HFHscanResultItem `json:"components"`
	Stage       int                 `json:"Stage"`
	Probability float32             `json:"probability"`
}

func HFHScanInit(s *zap.SugaredLogger, config *myconfig.ServerConfig) (*HFHscan, error) {
	//Initialize ldb tables
	urlTable, err := ldb.NewTableFromCfg(config.Ldb.Path, config.Ldb.KbName, "url", []string{"key", "component", "vendor", "version", "date", "license", "purl", "url", "a", "b", "c", "d", "e"}, false)
	if err != nil {
		return nil, fmt.Errorf("error creating urlTable: %v", err)
	}

	hfhTable, err := ldb.NewTableFromCfg(config.Ldb.Path, config.Ldb.KbName, "hfh", []string{"fileNames", "fileContents", "url"}, true)
	if err != nil {
		return nil, fmt.Errorf("error creating HFHtable: %v", err)
	}

	hfhSecTable, err := ldb.NewTableFromCfg(config.Ldb.Path, config.Ldb.KbName, "hfhSec", []string{"partialFileContents", "fileNames"}, true)
	if err != nil {
		return nil, fmt.Errorf("error creating HFHsecTable: %v", err)
	}

	return &HFHscan{hfhTable: hfhTable,
		urlTable:    urlTable,
		hfhSecTable: hfhSecTable,
		thStage1:    config.Hfh.Threshold1,
		thStage2:    config.Hfh.Threshold2,
		thStage3:    config.Hfh.Threshold3,
		dMax:        config.Hfh.Dmax,
		sectorTol:   config.Hfh.SectorTol,
		s:           s}, nil
}

// Scan is the main scanning function
func (s *HFHscan) Scan(input *dtos.HFHscanInput) (dtos.HFHResultOutput, error) {

	threshold := input.Threshold
	bestMatch := input.BestMatch
	if threshold <= 10 {
		threshold = 10
	}

	if threshold > 300 {
		threshold = 300
	}
	s.s.Infof("HFH threshold set: %.1f", threshold)

	s.thStage1 *= float32(threshold) / 100
	s.thStage2 *= float32(threshold) / 100
	s.thStage3 *= float32(threshold) / 100
	s.bestMatch = bestMatch
	s.resultsMap = make(map[string]HFHscanResult)
	projectTree := input.Root

	s.s.Infof("First stage starts")
	err := s.scanTreeFirstStage(projectTree)
	if err != nil {
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error during scan process fisrt stage %v", err)
	}

	s.s.Infof("Second stage starts")
	err = s.scanTreeSecondStage(projectTree)
	if err != nil {
		s.s.Error(err)
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error during scan process second stage %v", err)
	}

	jsonBytes, _ := json.Marshal(s.resultsMap)
	s.s.Debug(string(jsonBytes))

	s.s.Infof("Third stage starts")
	err = s.scanTreeThirdStage(projectTree)
	if err != nil {
		s.s.Error(err)
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error during scan process third stage %v", err)
	}

	jsonBytes, _ = json.Marshal(s.resultsMap)
	s.s.Debug(string(jsonBytes))

	s.s.Infof("Generating output")
	var results dtos.HFHResultOutput
	err = s.produceResults(projectTree, &results.Results)
	if err != nil {
		s.s.Error(err)
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error producing the response %v", err)
	}

	return results, nil
}

func hammingDistance(x, y uint64) int {
	xor := x ^ y
	return bits.OnesCount64(xor)
}

// scanHash queries the ldb tables and get the proximity candidates
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
		return nil, -1, nil, fmt.Errorf("error quering key %s - %v", hashHex, err)
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
	outputChan := make(chan []string, 100000*sectorTol)

	// Exec dump function in its own thread
	go func() {
		defer wg.Done()
		//_, err = table.Dump(start, end, -1, outputChan)
		table.DumpNative(start, end, -1, outputChan)
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
				if len(closestMatches) > 0 && key == closestMatches[len(closestMatches)-1] {
					continue
				}
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
	purlReleaseDate := make(map[string]time.Time)
	for _, urlKey := range urls {
		urls, err := table.QueryKey(urlKey)
		if err != nil {
			continue
		}
		for _, url := range urls {
			//extract the purl
			purl := url[6]
			//extract the version
			version := url[3]
			//exctract the release date
			date, err := time.Parse("2006-01-02", url[4])
			if err != nil {
				fmt.Printf("error parsing date: %s", err)
				continue
			}
			//use purl+version to track unique components
			if _, exist := uniquePurlVersions[purl+version]; exist {
				continue
			}
			uniquePurlVersions[purl+version] = true
			purlVersionMap[purl] = append(purlVersionMap[purl], version)
			existingDate, exists := purlReleaseDate[purl]
			if !exists || date.Before(existingDate) {
				purlReleaseDate[purl] = date
			}
		}
	}
	components := make([]HFHscanResultItem, len(purlVersionMap))
	i := 0
	for purl := range purlVersionMap {
		components[i].Purl = purl
		components[i].Versions = purlVersionMap[purl]
		components[i].Confidence = 100
		components[i].Date = purlReleaseDate[purl]
		i++
	}
	//sort by date
	sort.Slice(components, func(i, j int) bool {
		return components[i].Date.Before(components[j].Date)
	})
	return components
}

func (s *HFHscan) scanFirstStage(fileNamesSimhash string, fileContentsSimhash string) (*HFHscanResult, error) {

	//get the matching candidates from the HFH table based on the file names hash
	result, distance, content, err := scanHash(s.hfhTable, fileNamesSimhash, s.sectorTol, s.dMax)
	if err != nil {
		return nil, err
	}

	if len(result) > 0 {
		s.s.Debugf("FileNamesHash %s mathed with: %x with distance %d", fileNamesSimhash, result[0], distance)
	}
	//select the best based on the file contents hash
	minFilesContentdistance := s.dMax * 3 / 4
	if distance > s.dMax {
		return nil, fmt.Errorf("no results")
	}

	var lastKey uint64 = 0
	var bestMatches []uint64
	var bestUrlsKey []string
	//use the filecontents hash to select the best results
	for i, r := range result {
		if r == lastKey {
			s.s.Debugf("Skiping repeated key %x\n", r)
			continue
		}
		key, err := strconv.ParseUint(content[i][0], 16, 64)
		if err != nil {
			continue
		}
		lastKey = key
		contentsHash, err := strconv.ParseUint(fileContentsSimhash, 16, 64)
		if err != nil {
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
	//If we were able to find a good filecontent match we prefer it
	if len(bestUrlsKey) > 0 {
		s.s.Debugf("Filename Hash %s filecontent hash %s distance: %d - assigned URL: %s", fileNamesSimhash, fileContentsSimhash, minFilesContentdistance, bestUrlsKey)
		probability := (1 - float32(minFilesContentdistance)/float32(s.dMax)) * 100
		st := 1
		if probability < 0 {
			probability = (1 - float32(distance)/float32(s.dMax)) * 100
			st = 0
		}

		fistStageComponents := getComponents(s.urlTable, bestUrlsKey)
		return &HFHscanResult{Components: fistStageComponents, Probability: probability, Stage: st}, nil
	}
	//If not, process all the filenames hash
	s.s.Debugf("No filecontents match, using just fileNames match")
	probability := (1 - float32(distance)/float32(s.dMax)) * 100
	urls := make([]string, len(content))
	for i, c := range content {
		urls[i] = c[1]
	}
	fistStageComponents := getComponents(s.urlTable, urls)

	return &HFHscanResult{Components: fistStageComponents, Probability: probability, Stage: 0}, nil
}

func (s *HFHscan) scanTreeFirstStage(node *dtos.HFHScanInputChildren) error {

	result, err := s.scanFirstStage(node.SimHashNames, node.SimHashContent)
	if err != nil {
		return err
	}
	//If we matched a canditate using just the names, we are going to reduce that probability by half
	if result.Probability >= s.thStage1 {
		if result.Stage == 0 {
			result.Probability /= 2
		}
		s.resultsMap[node.PathId] = *result
	}

	if result.Probability < s.thStage1 || result.Stage == 0 {
		s.s.Debugf("probability lower than threshold (%.1f / %.1f), procesing node %s childs", result.Probability, s.thStage1, node.PathId)
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
	match, distance, content, err := scanHash(s.hfhSecTable, keyHex, s.sectorTol/4, s.dMax/3)
	if err != nil {
		return nil, err
	}

	s.s.Debugf("secHash: %x - matched: %x- distance %d\n", key, match, distance)
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

	probability := (1 - float32(distance)/float32(s.dMax)) * 100
	components := getComponents(s.urlTable, urlKeys)
	if len(components) == 0 {
		s.s.Debugf("no matched components for secHash %x", hash)
	}
	/*if len(components) > 1000 {
		s.s.Debugf("Ignore too popular hash, set prob to 0")
		probability = 0
	}*/

	return &HFHscanResult{Components: components, Probability: probability, Stage: 2}, nil
}

func (s *HFHscan) scanTreeSecondStage(node *dtos.HFHScanInputChildren) error {

	if s.resultsMap[node.PathId].Stage > 0 && s.resultsMap[node.PathId].Probability > s.thStage2 {
		s.s.Debugf("skiping children. Root node probability exceeds the threshold: %.1f/%.1f", s.resultsMap[node.PathId].Probability, s.thStage2)
		return nil
	}

	result, err := s.scanSecondStage(node.SimHashContent)
	if err != nil {
		return err
	}
	if result.Probability >= s.thStage2 && len(result.Components) > 0 {
		s.resultsMap[node.PathId] = *result
	}

	if s.resultsMap[node.PathId].Probability < s.thStage2 || s.resultsMap[node.PathId].Stage == 0 {
		for _, child := range node.Children {
			s.scanTreeSecondStage(child)
		}
	}

	return nil
}

func (s *HFHscan) scanTreeThirdStage(node *dtos.HFHScanInputChildren) error {
	if node.Children == nil {
		return nil
	}
	// If the matching probability is major than the TH we don't need to continue
	if s.resultsMap[node.PathId].Stage > 0 && s.resultsMap[node.PathId].Probability >= s.thStage3 {
		s.s.Debugf("skiping children. Root node probability exceeds the threshold: %.1f/%.1f", s.resultsMap[node.PathId].Probability, s.thStage3)
		return nil
	}

	// Go to the leaves first
	for _, child := range node.Children {
		s.s.Debugf("looking for leaves: %s -> %s", node.PathId, child.PathId)
		s.scanTreeThirdStage(child)
	}
	childPurlDate := make(map[string]time.Time)
	childPurlHits := make(map[string]int)
	childPurlProb := make(map[string]float32)
	allVersions := make(map[string]map[string]bool)
	childWithHits := 0

	for _, child := range node.Children {
		//ignore low prob results
		if len(s.resultsMap[child.PathId].Components) <= 0 { //|| child.Result.Prob < threshold {
			s.s.Debugf("ignore node without components: %s", child.PathId)
			continue
		}

		//rank the child purls
		for _, p := range s.resultsMap[child.PathId].Components {
			childWithHits++
			childPurlHits[p.Purl]++
			existingDate, exists := childPurlDate[p.Purl]
			if !exists || p.Date.Before(existingDate) {
				childPurlDate[p.Purl] = p.Date
			}
			if s.resultsMap[child.PathId].Stage == 3 {
				childs := float32(len(node.Children))
				childPurlProb[p.Purl] += p.Confidence / childs
			} else if s.resultsMap[child.PathId].Stage > 0 {
				childPurlProb[p.Purl] += s.resultsMap[child.PathId].Probability * (1 / float32(len(node.Children)))
			} else {
				childPurlProb[p.Purl] = -1
			}
			if _, ok := allVersions[p.Purl]; !ok {
				allVersions[p.Purl] = make(map[string]bool)
			}

			for _, version := range p.Versions {
				allVersions[p.Purl][version] = true
			}
		}
	}

	if len(childPurlHits) == 0 {
		return nil
	}

	sortedPurls := make([]string, 0, len(childPurlHits))
	for purl := range childPurlHits {
		sortedPurls = append(sortedPurls, purl)
	}

	// Ordenar el slice de purls basado en sus probabilidades en orden descendente
	sort.Slice(sortedPurls, func(i, j int) bool {
		if childPurlHits[sortedPurls[i]] != childPurlHits[sortedPurls[j]] {
			return childPurlHits[sortedPurls[i]] > childPurlHits[sortedPurls[j]]
		} else {
			return childPurlDate[sortedPurls[i]].Before(childPurlDate[sortedPurls[j]])
		}
	})

	s.s.Debugf("Sorted purls for %s: %s", node.PathId, sortedPurls[:])

	// Update the now results
	if len(sortedPurls) > 0 {
		eqprob := childPurlProb[sortedPurls[0]] // * float32(childPurlHits[sortedPurls[0]]) / float32(childWithHits) * (1 / float32(len(node.Children)))
		if eqprob > s.thStage3 {
			var newCOmponents []HFHscanResultItem
			for _, purl := range sortedPurls {
				var versions []string
				for v := range allVersions {
					versions = append(versions, v)
				}
				newCOmponent := HFHscanResultItem{Purl: purl, Versions: versions, Confidence: childPurlProb[purl]}
				newCOmponents = append(newCOmponents, newCOmponent)
			}
			nodeResults := s.resultsMap[node.PathId]
			nodeResults.Components = newCOmponents
			nodeResults.Probability = eqprob

			if s.resultsMap[node.PathId].Probability > 100.0 {
				s.s.Debugf("Warning prob %f bigger than 100.0%%\n", s.resultsMap[node.PathId].Probability)
				nodeResults.Probability = 100.0
			}
			nodeResults.Stage = 3
			s.resultsMap[node.PathId] = nodeResults

		} else {
			s.s.Debugf("%s: %s prob %f lower than threshold %.1f\n", node.PathId, sortedPurls, childPurlProb[sortedPurls[0]], s.thStage3)
		}
	}
	return nil
}

func (s *HFHscan) produceResults(node *dtos.HFHScanInputChildren, results *[]*dtos.HFHResult) error {
	result := s.resultsMap[node.PathId]
	if result.Probability > s.thStage3 && len(result.Components) > 0 {
		var components []*dtos.HFHComponent
		for _, c := range result.Components {
			components = append(components, &dtos.HFHComponent{Purl: c.Purl, Versions: c.Versions, Confidence: c.Confidence})
		}

		*results = append(*results, &dtos.HFHResult{PathId: node.PathId, Components: components})
		return nil
	}

	for _, child := range node.Children {
		s.produceResults(child, results)
	}

	return nil
}
