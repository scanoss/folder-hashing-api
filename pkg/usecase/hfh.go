package usecase

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/bits"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"time"

	"go.uber.org/zap"
	myconfig "scanoss.com/hfh-api/pkg/config"
	"scanoss.com/hfh-api/pkg/dtos"
	mv "scanoss.com/hfh-api/pkg/usecase/milvus"
	pp "scanoss.com/hfh-api/pkg/usecase/prefered_purls"
)

type HFHscan struct {
	Config         *HFHscanConfig
	resultsMap     map[string]HFHscanResult //the key is each folder, the content is the matched purl and probability
	namesContents  map[uint64]uint64        //map linking names and contents hash.
	nameHashPath   map[uint64]string        //map linking names hash with path
	nameHashLevels map[int][]uint64         //keeps the name hashes grouped by project structure level
	s              *zap.SugaredLogger
	thStage1       float32 //first stage threshold, if the probality is over the threshold is considered a valid match
	thStage2       float32 //second stage threshold, if the probality is over the threshold is considered a valid match
	thStage3       float32 //third stage threshold, if the probality is over the threshold is considered a valid match
	deepSearch     bool    //if true do not stop the scan when a folder is identified
}

// scan config structure, is defined when the service starts
type HFHscanConfig struct {
	mvDb             *mv.MilvusDb
	Dmax             int
	UrlsLimit        int
	ThStage1         float32
	ThStage2         float32
	ThStage3         float32
	preferedPurlList map[string]bool
}

type HFHscanResultItem struct {
	Purl     string    `json:"purl"`
	Versions []string  `json:"versions"`
	Rank     int32     `json:"rank"`
	Date     time.Time `json:"date"`
}

type HFHscanResult struct {
	Components  []HFHscanResultItem `json:"components"`
	Stage       int                 `json:"Stage"`
	Probability float32             `json:"probability"`
}

const (
	reportedVersionsNumber = 5
	purlRankDefaultValue   = 5
)

// Scanning module initialization
func HFHScanInit(config *myconfig.ServerConfig, testMode bool) *HFHscanConfig {
	scanner := HFHscanConfig{
		ThStage1:  config.Hfh.Threshold1,
		ThStage2:  config.Hfh.Threshold2,
		ThStage3:  config.Hfh.Threshold3,
		Dmax:      config.Hfh.Dmax,
		UrlsLimit: config.Hfh.UrlsLimit,
	}

	var err error

	scanner.preferedPurlList, err = pp.InitPurlMap(config.Hfh.CuratedPurlListPath)
	if err != nil {
		log.Printf("Prefered purl list couldn't be loaded: %s", err)
	}

	dbName := ""
	if testMode {
		dbName = "test"
	}
	//new milvus db with default config, if milvus is not available the service cannot start
	scanner.mvDb, err = mv.NewMilvusDb("", "", dbName)
	if err != nil {
		return nil
	}

	return &scanner
}

// new scan request
func HFHScanNew(log *zap.SugaredLogger, config *HFHscanConfig, input *dtos.HFHscanInput) *HFHscan {
	scanner := HFHscan{s: log, resultsMap: make(map[string]HFHscanResult), nameHashPath: make(map[uint64]string),
		namesContents: make(map[uint64]uint64), Config: config, nameHashLevels: make(map[int][]uint64)}
	threshold := input.Threshold
	bestMatch := input.BestMatch

	//threshold adjustment
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
	scanner.deepSearch = bestMatch //TODO: update papi definition
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

	/*jsonBytes, _ := json.Marshal(s.resultsMap)
	s.s.Debug(string(jsonBytes))*/

	s.s.Infof("--- Purl Grouping --- \n")
	err = s.scanTreeThirdStage(projectTree)
	if err != nil {
		s.s.Error(err)
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error during scan process third stage %v", err)
	}

	/*jsonBytes, _ = json.Marshal(s.resultsMap)
	s.s.Debug(string(jsonBytes))*/

	s.s.Infof("Generating output")
	var results dtos.HFHResultOutput
	err = s.produceResults(projectTree, &results.Results)
	if err != nil {
		s.s.Error(err)
		return dtos.HFHResultOutput{}, fmt.Errorf("unexpected error producing the response %v", err)
	}

	return results, nil
}

// hamming distance calculation
func hammingDistance(x, y uint64) int {
	xor := x ^ y
	return bits.OnesCount64(xor)
}

// helper function to parse purl. TODO: this must be improved
func parsePurl(purl string) (string, string, string, error) {
	// Check if the purl is empty
	if purl == "" {
		return "", "", "", errors.New("PURL cannot be empty")
	}

	// Check if the purl starts with "pkg:"
	if !strings.HasPrefix(purl, "pkg:") {
		return "", "", "", errors.New("PURL must start with \"pkg:\"")
	}

	// Remove the "pkg:" prefix and split the rest by "/"
	parts := strings.Split(strings.TrimPrefix(purl, "pkg:"), "/")

	// Check that there are exactly 3 parts (source, vendor, component)
	if len(parts) != 3 {
		return "", "", "", errors.New("PURL must have the format \"pkg:source/vendor/component\"")
	}

	// Extract the components
	source := parts[0]
	vendor := parts[1]
	component := parts[2]

	// Check that none of the components is empty
	if source == "" || vendor == "" || component == "" {
		return "", "", "", errors.New("source, vendor, and component cannot be empty")
	}

	// Return the three strings
	return source, vendor, component, nil
}

// helper function to rank purls. TODO: this must be improved
func purlRank(purl string, startRank int32) int32 {
	rank := startRank
	source, vendor, component, err := parsePurl(purl)
	if err == nil {
		if source == "github" {
			rank--
		}
		if vendor == component {
			rank--
		}
		return rank
	}
	return 10
}

// retrieves purl information from milvus and sort by date and extension
func (s *HFHscan) getComponents(urls []uint64, limit int) []HFHscanResultItem {
	uniquePurls := make(map[string]bool)
	uniquePurlVersions := make(map[string]bool)
	purlVersionMap := make(map[string][]string)
	purlUrlMap := make(map[string]string)
	purlReleaseDate := make(map[string]time.Time)
	uniquePurlsLimit := limit
	for _, urlKey := range urls {
		urlRecord, err := s.Config.mvDb.GetComponent(urlKey)
		if err != nil {
			continue
		}
		if len(uniquePurls) > uniquePurlsLimit {
			break
		}
		//extract the purl
		purl := urlRecord[0]
		//extract the version
		version := urlRecord[1]
		//exctract the release date
		date, err := time.Parse("2006-01-02", urlRecord[2])
		if err != nil {
			s.s.Errorf("error parsing date: %s", err)
			continue
		}
		//use purl+version to track unique components
		if _, exist := uniquePurlVersions[purl+version]; exist {
			continue
		}

		url := urlRecord[3]

		uniquePurls[purl] = true
		uniquePurlVersions[purl+version] = true
		purlVersionMap[purl] = append(purlVersionMap[purl], version)
		purlUrlMap[purl] = url
		existingDate, exists := purlReleaseDate[purl]
		if !exists || date.Before(existingDate) {
			purlReleaseDate[purl] = date
		}
		uniquePurlsLimit--

	}
	if len(purlVersionMap) > 0 {
		components := make([]HFHscanResultItem, len(purlVersionMap))
		i := 0
		for purl := range purlVersionMap {
			components[i].Purl = purl
			components[i].Versions = purlVersionMap[purl]
			if len(components[i].Versions) > reportedVersionsNumber {
				components[i].Versions = components[i].Versions[:reportedVersionsNumber]
			}
			//rank component
			var rank int32 = purlRankDefaultValue
			//no source code are penalized
			if !strings.HasSuffix(purlUrlMap[purl], ".zip") && !strings.HasSuffix(purlUrlMap[purl], ".tar.gz") {
				rank++
			}

			components[i].Rank = purlRank(purl, rank)
			components[i].Date = purlReleaseDate[purl]
			i++
		}
		//sort by date
		sort.Slice(components, func(i, j int) bool {
			return components[i].Date.Before(components[j].Date)
		})

		return components
	}
	return nil
}

type HashCount struct {
	Hash  uint64
	Count int
}

// RankHashesByColumns takes a matrix of hashes and returns a sorted slice
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
			foundMatch := false

			if hash == 0 {
				continue
			}

			// Compare only with subsequent rows
			for otherRow := row + 1; otherRow < len(matrix); otherRow++ {
				if matrix[otherRow] == nil {
					continue
				}

				// Compare with all elements in the other row
				for l := 0; l < len(matrix[otherRow]); l++ {
					otherHash := matrix[otherRow][l]
					if otherHash == 0 {
						continue
					}

					distance := hammingDistance(hash, otherHash)
					if distance <= threshold {
						hashCounts[hash]++
						foundMatch = true
						break
					}
				}

				// If match found in this row, skip to next row
				if foundMatch {
					break
				}
			}
			if foundMatch {
				break
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

// initilizate the auxiliar maps
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

// At the first stage we travel the project levels, sending to milvus all the hashes of each level and processing the results
func (s *HFHscan) scanTreeFirstStage(node *dtos.HFHScanInputChildren) error {
	rootLevel := 0
	s.initMap(node, &rootLevel)
	//maxLevel means how deep we are going to go in the map levels
	maxLevel := 2 * int(math.Sqrt(float64(len(s.nameHashLevels))))
	if maxLevel < 2 {
		maxLevel = 2
	}
	//if deepSeach is enabled we travel all the maps levels
	if s.deepSearch {
		maxLevel = len(s.nameHashLevels)
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
		distances, urls, err := s.Config.mvDb.Mainsearch(nameHashes, contentHashes, 0, s.Config.preferedPurlList)
		if err != nil {
			return err
		}
		//process results
		for i, d := range distances {
			probability := (1 - float32(d)/float32(s.Config.Dmax)) * 100
			fistStageComponents := s.getComponents(urls[i], s.Config.UrlsLimit)
			s.resultsMap[s.nameHashPath[nameHashes[i]]] = HFHscanResult{Components: fistStageComponents, Probability: probability, Stage: 1}
		}
	}
	return nil
}

// At the second stage we look por partial dependencies using the secondary hash table.
// Go to each node without results, and if it has more than 3 subfolders, try to match this subfolders to a component
func (s *HFHscan) scanTreeSecondStage(node *dtos.HFHScanInputChildren) error {

	if s.resultsMap[node.PathId].Probability > s.thStage1 {
		return nil
	}
	s.s.Debugf("Procesing node %s\n", node.PathId)
	var contentHashes []uint64
	if len(node.Children) > 3 {
		for _, child := range node.Children {
			contentHash, _ := strconv.ParseUint(child.SimHashContent, 16, 64)
			contentHashes = append(contentHashes, contentHash)
		}
		//query milvus fusing the subfolders content hashes
		resultMatrix, err := s.Config.mvDb.SecondarySearch(contentHashes, 1)
		if err != nil {
			return err
		}
		//process the results loocking for a component in common
		ranking := RankHashesByColumns(resultMatrix, 2)
		eqProb := float32(0)
		if len(ranking) > 0 {
			eqProb = float32(ranking[0].Count) * 100 / float32(len(node.Children))
		}
		if eqProb >= s.thStage2 {
			var urlKeys []uint64
			distance := 0
			for i, r := range ranking {
				//go to the main hfh table to look for the url hash
				if i > 0 && r.Count < ranking[i-1].Count {
					break
				}
				contentHash, _ := strconv.ParseUint(node.SimHashContent, 16, 64)
				d, urls, err := s.Config.mvDb.Mainsearch([]uint64{r.Hash}, []uint64{contentHash}, 0, s.Config.preferedPurlList)
				urlKeys = urls[0]
				distance = d[0]
				if err != nil {
					fmt.Println(err)
					continue
				}
			}
			if len(urlKeys) > 0 {
				s.s.Infof("%s matched distance: %d\n", node.PathId, distance)
				components := s.getComponents(urlKeys, s.Config.UrlsLimit)
				if eqProb > 100.0 {
					eqProb = 100.0
				}
				//	namesHash, _ := strconv.ParseUint(node.SimHashNames, 16, 64)
				//	dist := hammingDistance(namesHash, ranking[0].Hash)
				probability := eqProb //(1 - float32(dist)/(float32(s.Config.Dmax))) * 100
				//	probability = (probability + eqProb) / 2
				s.resultsMap[node.PathId] = HFHscanResult{Components: components, Probability: probability, Stage: 2}
				if probability > s.thStage2 {
					return nil
				}
			}
		}
	}
	for _, child := range node.Children {
		s.scanTreeSecondStage(child)
	}
	return nil
}

// At third stage we try to group subfolders components.
// If three subfolders have the same component, then this fathers must be assigned to the same one
func (s *HFHscan) scanTreeThirdStage(node *dtos.HFHScanInputChildren) error {
	if node.Children == nil {
		return nil
	}
	// If the matching probability is major than the TH we don't need to continue
	if s.resultsMap[node.PathId].Stage > 0 && s.resultsMap[node.PathId].Probability >= s.thStage3 && !s.deepSearch {
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
			if s.resultsMap[child.PathId].Stage > 0 {
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
	preferedFound := false

	if len(preferedPurls) > 0 {
		sortedPurls = preferedPurls
		preferedFound = true
	} else {
		sortedPurls = normalPurls
	}

	// Sort based on purls dates
	sort.Slice(sortedPurls, func(i, j int) bool {
		if childPurlHits[sortedPurls[i]] != childPurlHits[sortedPurls[j]] {
			return childPurlHits[sortedPurls[i]] > childPurlHits[sortedPurls[j]]
		} else {
			return childPurlDate[sortedPurls[i]].Before(childPurlDate[sortedPurls[j]])
		}
	})

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
				var rank int32 = 1
				if !preferedFound {
					rank = purlRank(purl, purlRankDefaultValue)
				}
				newCOmponent := HFHscanResultItem{Purl: purl, Versions: versions, Rank: rank}
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
	if result.Probability > s.thStage3 && len(result.Components) > 0 {
		var components []*dtos.HFHComponent
		var preferedcomponents []*dtos.HFHComponent

		for _, c := range result.Components {
			if s.Config.preferedPurlList[c.Purl] {
				preferedcomponents = append(preferedcomponents, &dtos.HFHComponent{Purl: c.Purl, Versions: c.Versions, Rank: 1})
			} else {
				rank := purlRank(c.Purl, 5)
				components = append(components, &dtos.HFHComponent{Purl: c.Purl, Versions: c.Versions, Rank: rank})
			}
		}
		if len(preferedcomponents) > 0 {
			*results = append(*results, &dtos.HFHResult{PathId: node.PathId, Components: preferedcomponents, Probability: result.Probability, Stage: result.Stage})
		} else {
			limit := 10
			if len(components) < limit {
				limit = len(components)
			}
			var prob float32 = result.Probability
			if result.Stage == 2 {
				prob = result.Probability/(float32(len(components)%5)) + 1
			}
			*results = append(*results, &dtos.HFHResult{PathId: node.PathId, Components: components[:limit], Probability: prob, Stage: result.Stage})
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
