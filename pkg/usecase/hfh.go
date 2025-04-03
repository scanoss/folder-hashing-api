package usecase

import (
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"path/filepath"
	"sort"
	"strconv"

	"time"

	"go.uber.org/zap"
	myconfig "scanoss.com/hfh-api/pkg/config"
	"scanoss.com/hfh-api/pkg/dtos"
	ldb "scanoss.com/hfh-api/pkg/usecase/ldb"
	mv "scanoss.com/hfh-api/pkg/usecase/milvus"
	pp "scanoss.com/hfh-api/pkg/usecase/prefered_purls"
)

type HFHscan struct {
	Config         *HFHscanConfig
	resultsMap     map[string]HFHscanResult
	namesContents  map[uint64]uint64
	nameHashPath   map[uint64]string
	nameHashLevels map[int][]uint64
	s              *zap.SugaredLogger
	thStage1       float32
	thStage2       float32
	thStage3       float32
	deepSearch     bool
	bestMatch      bool
}

type HFHscanConfig struct {
	HfhTable         *ldb.TableDefinition
	HfhSecTable      *ldb.TableDefinition
	UrlTable         *ldb.TableDefinition
	mvDb             *mv.MilvusDb
	Dmax             int
	SectorTol        int
	UrlsLimit        int
	ThStage1         float32
	ThStage2         float32
	ThStage3         float32
	preferedPurlList map[string]bool
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

const (
	reportedVersionsNumber = 5
)

func HFHScanInit(config *myconfig.ServerConfig) *HFHscanConfig {
	scanner := HFHscanConfig{
		ThStage1:  config.Hfh.Threshold1,
		ThStage2:  config.Hfh.Threshold2,
		ThStage3:  config.Hfh.Threshold3,
		Dmax:      config.Hfh.Dmax,
		SectorTol: config.Hfh.SectorTol,
		UrlsLimit: config.Hfh.UrlsLimit,
	}

	var err error
	//Initialize ldb tables
	scanner.UrlTable, err = ldb.NewTableFromCfg(config.Ldb.BinaryPath, config.Ldb.KbName, "url", []string{"key", "component", "vendor", "version", "date", "license", "purl", "url", "a", "b", "c", "d", "e"}, false)
	if err != nil {
		return nil
	}

	scanner.HfhTable, err = ldb.NewTableFromCfg(config.Ldb.BinaryPath, config.Ldb.KbName, "hfh", []string{"fileNames", "fileContents", "url"}, true)
	if err != nil {
		return nil
	}
	scanner.HfhSecTable, err = ldb.NewTableFromCfg(config.Ldb.BinaryPath, config.Ldb.KbName, "hfhSec", []string{"partialFileContents", "fileNames"}, true)
	if err != nil {
		return nil
	}

	scanner.preferedPurlList, _ = pp.InitPurlMap(config.Hfh.CuratedPurlListPath)
	//new milvus db with default config
	scanner.mvDb, err = mv.NewMilvusDb("", "")
	if err != nil {
		return nil
	}

	return &scanner
}

func HFHScanNew(log *zap.SugaredLogger, config *HFHscanConfig, input *dtos.HFHscanInput) *HFHscan {
	scanner := HFHscan{s: log, resultsMap: make(map[string]HFHscanResult), nameHashPath: make(map[uint64]string),
		namesContents: make(map[uint64]uint64), Config: config, nameHashLevels: make(map[int][]uint64)}
	threshold := input.Threshold
	bestMatch := input.BestMatch
	if threshold <= 10 {
		threshold = 10
	}

	if threshold > 300 {
		threshold = 300
	}
	scanner.s.Infof("HFH threshold set: %d%%", threshold)

	scanner.thStage1 = scanner.Config.ThStage1 * float32(threshold) / 100
	scanner.thStage2 = scanner.Config.ThStage2 * float32(threshold) / 100
	scanner.thStage3 = scanner.Config.ThStage3 * float32(threshold) / 100
	scanner.bestMatch = bestMatch
	scanner.deepSearch = false
	return &scanner
}

// Scan is the main scanning function
func (s *HFHscan) Scan(root *dtos.HFHScanInputChildren) (dtos.HFHResultOutput, error) {
	projectTree := root
	if s.Config.preferedPurlList == nil {
		s.s.Warnf("curated purl list couln't be loaded")
	}

	s.s.Infof("--- First stage starts --- \n")
	err := s.scanTreeFirstStage(projectTree)
	if err != nil {
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error during scan process fisrt stage %v", err)
	}

	s.s.Infof("--- Second stage starts --- \n")
	err = s.scanTreeSecondStage(projectTree)
	if err != nil {
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error during scan process fisrt stage %v", err)
	}

	jsonBytes, _ := json.Marshal(s.resultsMap)
	s.s.Debug(string(jsonBytes))

	s.s.Infof("--- Purl Grouping --- \n")
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

// retrieves purl information from the url table
func getComponents(table *ldb.TableDefinition, urls []string, limit int) []HFHscanResultItem {
	uniquePurls := make(map[string]bool)
	uniquePurlVersions := make(map[string]bool)
	purlVersionMap := make(map[string][]string)
	purlReleaseDate := make(map[string]time.Time)
	uniquePurlsLimit := limit
	for _, urlKey := range urls {
		urlRecords, err := table.QueryKey(urlKey)
		if err != nil {
			continue
		}
		if len(uniquePurls) > uniquePurlsLimit {
			break
		}
		for _, url := range urlRecords {
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
			uniquePurls[purl] = true
			uniquePurlVersions[purl+version] = true
			purlVersionMap[purl] = append(purlVersionMap[purl], version)
			existingDate, exists := purlReleaseDate[purl]
			if !exists || date.Before(existingDate) {
				purlReleaseDate[purl] = date
			}
			uniquePurlsLimit--
		}
	}
	if len(purlVersionMap) > 0 {
		components := make([]HFHscanResultItem, len(purlVersionMap))
		i := 0
		for purl := range purlVersionMap {
			components[i].Purl = purl
			components[i].Versions = purlVersionMap[purl]
			if len(components[i].Versions) > 5 {
				components[i].Versions = components[i].Versions[:reportedVersionsNumber]
			}
			components[i].Confidence = 100.0 / float32(len(uniquePurls))
			components[i].Date = purlReleaseDate[purl]
			i++
		}
		//sort by date
		sort.Slice(components, func(i, j int) bool {
			return components[i].Date.Before(components[j].Date)
		})
		/*if len(components) > 10 {
			return components[:limit/10]
		}*/
		return components
	}
	return nil
}

/*
func (s *HFHscan) scanFirstStage(fileNamesSimhash string, fileContentsSimhash string) (*HFHscanResult, error) {

	NamesHash, _ := strconv.ParseUint(fileNamesSimhash, 16, 64)
	contentHash, _ := strconv.ParseUint(fileContentsSimhash, 16, 64)
	d, urls, err := s.Config.mvDb.Mainsearch([]uint64{NamesHash}, []uint64{contentHash}, s.Config.Dmax)

	if err != nil {
		log.Println(err)
	}

	if len(urls) > 0 && len(urls[0]) > 0 {
		fistStageComponents := getComponents(s.Config.UrlTable, urls[0], s.Config.UrlsLimit)
		probability := (1 - float32(d[0])/float32(s.Config.Dmax)) * 100
		return &HFHscanResult{Components: fistStageComponents, Probability: probability, Stage: 1}, nil
	}

	return nil, nil
}*/

type HashCount struct {
	Hash  uint64
	Count int
}

func hammingDistanceString(hash1, hash2 string) int {
	x, _ := strconv.ParseUint(hash1, 16, 64)
	y, _ := strconv.ParseUint(hash2, 16, 64)
	xor := x ^ y
	return bits.OnesCount64(xor)
}

// RankHashesByColumns takes a matrix of strings (hashes) and returns a sorted slice
// of HashCount where Count represents the number of different columns where each hash appears
func RankHashesByColumns(matrix [][]uint64, threshold int) []HashCount {
	if len(matrix) == 0 {
		return []HashCount{}
	}

	// Map to store the count for each hash
	hashCounts := make(map[uint64]int)

	// Add existent hashes with count 1
	for i := 0; i < len(matrix); i++ {
		if matrix[i] == nil {
			continue
		}
		for j := 0; j < len(matrix[i]); j++ {
			hash := matrix[i][j]
			if hash == 0 {
				continue
			}
			if _, exists := hashCounts[hash]; !exists {
				hashCounts[hash] = 1
			}
		}
	}

	// Compare hashes only with elements from different rows
	for row := 0; row < len(matrix); row++ {
		if matrix[row] == nil {
			continue
		}

		// For each hash in current row
		for j := 0; j < len(matrix[row]); j++ {
			hash := matrix[row][j]
			if hash == 0 {
				continue
			}

			// Compare only with subsequent rows
			for otherRow := row + 1; otherRow < len(matrix); otherRow++ {
				if matrix[otherRow] == nil {
					continue
				}

				foundMatch := false
				// Compare with all elements in the other row
				for l := 0; l < len(matrix[otherRow]); l++ {
					otherHash := matrix[otherRow][l]
					if otherHash == 0 {
						continue
					}

					distance := hammingDistance(hash, otherHash)
					if distance <= threshold {
						if hash == otherHash {
							// If exact match, increment only one
							hashCounts[hash]++
						} else {
							// If different but within threshold, increment both
							hashCounts[hash]++
							hashCounts[otherHash]++
						}
						foundMatch = true
						break
					}
				}

				// If match found in this row, skip to next row
				if foundMatch {
					break
				}
			}
		}
	}

	// Convert to ranking slice
	ranking := make([]HashCount, 0, len(hashCounts))
	for hash, count := range hashCounts {
		ranking = append(ranking, HashCount{
			Hash:  hash,
			Count: count,
		})
	}

	// Sort by count (highest to lowest) and then by hash (alphabetically)
	sort.Slice(ranking, func(i, j int) bool {
		if ranking[i].Count == ranking[j].Count {
			return ranking[i].Hash < ranking[j].Hash
		}
		return ranking[i].Count > ranking[j].Count
	})

	return ranking

}

func (s *HFHscan) initMap(node *dtos.HFHScanInputChildren, level *int) error {

	mLevel := *level
	namesHash, _ := strconv.ParseUint(node.SimHashNames, 16, 64)
	contentHash, _ := strconv.ParseUint(node.SimHashContent, 16, 64)
	if s.nameHashPath[namesHash] == "" {
		s.nameHashPath[namesHash] = node.PathId
		s.namesContents[namesHash] = contentHash
		s.nameHashLevels[mLevel] = append(s.nameHashLevels[mLevel], namesHash)
	}

	mLevel++
	for _, child := range node.Children {
		s.initMap(child, &mLevel)
	}
	return nil

}

func (s *HFHscan) scanTreeFirstStage(node *dtos.HFHScanInputChildren) error {
	rootLevel := 0
	s.initMap(node, &rootLevel)
	maxLevel := 2 * int(math.Sqrt(float64(len(s.nameHashLevels))))
	if maxLevel < 2 {
		maxLevel = 2
	}
	for level := 0; level < maxLevel; level++ {
		if s.nameHashLevels[level] == nil {
			break
		}
		s.s.Debugf("Procesing level... %d/%d\n", level, maxLevel)
		var nameHashes []uint64
		//for fist level process also the root
		//check if the dir father was already matched
		for _, h := range s.nameHashLevels[level] {
			child := s.nameHashPath[h]
			skip := false
			for father := filepath.Dir(child); father != child; {
				if s.resultsMap[father].Probability > s.thStage1 {
					skip = true
					break
				}
				child = father
				father = filepath.Dir(child)
			}
			if skip {
				continue
			}
			nameHashes = append(nameHashes, h)
		}

		var contentHashes []uint64
		for _, h := range nameHashes {
			contentHashes = append(contentHashes, s.namesContents[h])
		}
		//look for coincidences
		distances, urls, err := s.Config.mvDb.Mainsearch(nameHashes, contentHashes, s.Config.Dmax)
		if err != nil {
			return err
		}
		//process results
		for i, d := range distances {
			probability := (1 - float32(d)/float32(s.Config.Dmax)) * 100
			fistStageComponents := getComponents(s.Config.UrlTable, urls[i], s.Config.UrlsLimit)
			s.resultsMap[s.nameHashPath[nameHashes[i]]] = HFHscanResult{Components: fistStageComponents, Probability: probability, Stage: 1}
		}
	}
	return nil

	/*for n, c := range s.namesContents {
		nameHashes = append(nameHashes, n)
		contentHashes = append(contentHashes, c)
	}

	distances, urls, err := s.Config.mvDb.Mainsearch(nameHashes, contentHashes)
	if err != nil {
		return err
	}
	for i, d := range distances {
		probability := (1 - float32(d)/float32(s.Config.Dmax)) * 100
		if probability > s.thStage1 {
			fistStageComponents := getComponents(s.Config.UrlTable, urls[i], s.Config.UrlsLimit)
			s.resultsMap[s.nameHashPath[nameHashes[i]]] = HFHscanResult{Components: fistStageComponents, Probability: probability, Stage: 1}
		}
	}*/
	return nil
}

/* Calc hash head */
func headCalc(simHash uint64) byte {
	var sum int
	for i := 0; i < 8; i++ {
		b := byte((simHash >> (i * 8)) & 0xFF)
		sum += int(b) * 2
	}
	return byte(sum >> 4 & 0xFF)
}

func (s *HFHscan) scanTreeSecondStage(node *dtos.HFHScanInputChildren) error {

	if s.resultsMap[node.PathId].Probability > s.thStage1 {
		return nil
	}
	s.s.Debugf("Procesing node %s\n", node.PathId)
	if len(node.Children) > 2 {
		var contentHashes []uint64

		for _, child := range node.Children {
			s.s.Debugf(child.PathId)
			contentHash, _ := strconv.ParseUint(child.SimHashContent, 16, 64)
			/*			head := headCalc(contentHash)
						//Overwrite the MS byte with the head to keep the hash size total
						contentHash = (contentHash & 0x00FFFFFFFFFFFFFF) | (uint64(head) << 56)*/
			contentHashes = append(contentHashes, contentHash)
		}
		resultMatrix, err := s.Config.mvDb.SecondarySearch(contentHashes, s.Config.Dmax/5)
		if err != nil {
			return err
		}
		ranking := RankHashesByColumns(resultMatrix, s.Config.Dmax*25/100)
		eqProb := float32(0)
		if len(ranking) > 0 {
			eqProb = float32(ranking[0].Count) * 100 / float32(len(node.Children)*ranking[len(ranking)-1].Count)
		}
		if eqProb >= s.thStage2 {
			var urlKeys []string
			for i, r := range ranking {
				//go to the main hfh table to look for the url hash
				if i > 0 && r.Count < ranking[i-1].Count {
					break
				}
				contentHash, _ := strconv.ParseUint(node.SimHashContent, 16, 64)
				_, urls, err := s.Config.mvDb.Mainsearch([]uint64{r.Hash}, []uint64{contentHash}, s.Config.Dmax*2)
				urlKeys = urls[0]
				if err != nil {
					fmt.Println(err)
					continue
				}
			}
			if len(urlKeys) > 0 {
				components := getComponents(s.Config.UrlTable, urlKeys, s.Config.UrlsLimit)
				if eqProb > 100.0 {
					eqProb = 100.0
				}
				s.resultsMap[node.PathId] = HFHscanResult{Components: components, Probability: eqProb, Stage: 2}
				return nil
			}
		}
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
	if s.resultsMap[node.PathId].Stage > 0 && s.resultsMap[node.PathId].Probability >= s.thStage3 && !s.bestMatch {
		s.s.Debugf("skiping children. Root node %s probability exceeds the threshold: %.1f/%.1f", node.PathId, s.resultsMap[node.PathId].Probability, s.thStage3)
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
	childWithoutComponents := 0
	s.s.Debugf("Now at %s", node.PathId)

	for _, child := range node.Children {
		//ignore low prob results
		if len(s.resultsMap[child.PathId].Components) <= 0 { //|| child.Result.Prob < threshold {
			s.s.Debugf("ignore node without components: %s", child.PathId)
			childWithoutComponents++
			continue
		}
		s.s.Debugf("child %s of %s", child.PathId, node.PathId)
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
				childPurlProb[p.Purl] += s.resultsMap[child.PathId].Probability * (1 / float32(len(node.Children)-childWithoutComponents))
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
	preferedPurls := make([]string, 0)
	normalPurls := make([]string, 0)
	for purl := range childPurlHits {
		if s.Config.preferedPurlList[purl] {
			preferedPurls = append(preferedPurls, purl)
		}
		normalPurls = append(normalPurls, purl)
	}

	var sortedPurls []string
	if len(preferedPurls) > 0 {
		sortedPurls = preferedPurls
	} else {
		sortedPurls = normalPurls
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
			for i, purl := range sortedPurls {
				var versions []string
				versionNumber := 0
				for _, version := range allVersions {
					for v := range version {
						versions = append(versions, v)
						versionNumber++
					}
					if versionNumber > reportedVersionsNumber {
						break
					}
				}
				newCOmponent := HFHscanResultItem{Purl: purl, Versions: versions, Confidence: childPurlProb[purl]}
				newCOmponents = append(newCOmponents, newCOmponent)
				if i > s.Config.UrlsLimit {
					break
				}
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
	if result.Probability > s.thStage2 && len(result.Components) > 0 {
		var components []*dtos.HFHComponent
		var preferedcomponents []*dtos.HFHComponent

		for _, c := range result.Components {
			if s.Config.preferedPurlList[c.Purl] {
				preferedcomponents = append(preferedcomponents, &dtos.HFHComponent{Purl: c.Purl, Versions: c.Versions, Confidence: result.Probability})
			} else {
				components = append(components, &dtos.HFHComponent{Purl: c.Purl, Versions: c.Versions, Confidence: c.Confidence * result.Probability / 100})
			}
		}
		if len(preferedcomponents) > 0 {
			*results = append(*results, &dtos.HFHResult{PathId: node.PathId, Components: preferedcomponents, Probability: result.Probability, Stage: result.Stage})
		} else if components[0].Confidence > s.thStage3/3 {
			limit := 10
			if len(components) < limit {
				limit = len(components)
			}
			*results = append(*results, &dtos.HFHResult{PathId: node.PathId, Components: components[:limit], Probability: result.Probability, Stage: result.Stage})
		}

		if len(node.Children) == 0 {
			return nil
		}
		purlRank := make(map[string]int)
		for _, child := range node.Children {
			for _, fatherComp := range s.resultsMap[node.PathId].Components {
				for _, childComp := range s.resultsMap[child.PathId].Components {
					if fatherComp.Purl == childComp.Purl {
						purlRank[fatherComp.Purl]++
					}
				}
			}
		}
		for _, count := range purlRank {
			if float32(count)*100/float32(len(node.Children)) > s.thStage3 {
				return nil
			}
		}
	}

	for _, child := range node.Children {
		s.produceResults(child, results)
	}
	return nil
}
