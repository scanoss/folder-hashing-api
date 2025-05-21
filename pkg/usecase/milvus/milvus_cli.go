package milvus

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"go.uber.org/zap"
)

var (
	mainColletionName      = "url"
	secondaryColletionName = "secondary"
	defaultTopResults      = 10000

	OutputFieldNamesMain = []string{"hfhNames", "hfhContents", "urlHash"}
	OutputFieldNamesSec  = []string{"hfhContents", "hfhNames"}
	defaultHost          = "localhost"
	defaultPort          = "19530"
)

type MilvusDb struct {
	address       string
	databaseName  string
	s             *zap.SugaredLogger
	TopMainResult int
}

func NewMilvusDb(host string, port string, database string) (*MilvusDb, error) {

	if host == "" {
		host = defaultHost
	}

	if port == "" {
		port = defaultPort
	}

	if database == "" {
		database = "default"
	}
	// Milvus GRPC connection
	milvusAddress := fmt.Sprintf("%s:%s", host, port)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s := ctxzap.Extract(ctx).Sugar()

	c, err := client.NewGrpcClient(ctx, milvusAddress)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	s.Info("Sucessfuly connected to Milvus KB")
	err = c.UsingDatabase(ctx, database)
	if err != nil {
		return nil, err
	}

	s.Infof("Using %s database", database)

	// Check if the collections are available
	has, err := c.HasCollection(ctx, mainColletionName)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, fmt.Errorf("the main collection is not available")
	}
	err = c.LoadCollection(ctx, mainColletionName, true)
	if err != nil {
		return nil, fmt.Errorf("error al cargar colección: %v", err)
	}

	has, err = c.HasCollection(ctx, secondaryColletionName)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, fmt.Errorf("the secondary collection is not available")
	}

	err = c.LoadCollection(ctx, secondaryColletionName, true)
	if err != nil {
		return nil, fmt.Errorf("error al cargar colección: %v", err)
	}
	return &MilvusDb{s: s, address: milvusAddress, TopMainResult: defaultTopResults, databaseName: database}, nil
}

func (db *MilvusDb) Mainsearch(mainHashes []uint64, secHashes []uint64, topResults int, topPurls map[string]bool) ([]int, [][]uint64, error) {

	outputFields := []string{"hfhDirs", "urlHash", "purl"}

	if topResults <= 0 {
		topResults = db.TopMainResult
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	c, err := client.NewGrpcClient(ctx, db.address)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close()

	err = c.UsingDatabase(ctx, db.databaseName)
	if err != nil {
		return nil, nil, err
	}
	// Make sure input slices have the same length
	if len(mainHashes) != len(secHashes) {
		return nil, nil, fmt.Errorf("mainHashes and secHashes must have the same length")
	}

	// Initialize result slices
	matchedDistances := make([]int, len(mainHashes))
	matchedUrls := make([][]uint64, len(mainHashes))

	// Default all distances to 999 (indicating no match)
	for i := range matchedDistances {
		matchedDistances[i] = 999
	}

	// Process in blocks of 20
	blockSize := 20
	for start := 0; start < len(mainHashes); start += blockSize {
		// Calculate end index for current block
		end := start + blockSize
		if end > len(mainHashes) {
			end = len(mainHashes)
		}

		// Get the current block
		currentMainHashes := mainHashes[start:end]
		currentSecHashes := secHashes[start:end]

		// Search for the current block
		searchResults, err := searchSimilarHashes(ctx, c, currentMainHashes, topResults, 10,
			mainColletionName, "hfhNames", outputFields, []string{"github_popular", "github"})
		if err != nil {
			return nil, nil, err
		}

		// Process the results for this block
		for i, result := range searchResults {
			// Calculate the original index
			originalIndex := start + i
			db.s.Debugf("%x #%d matches found\n", mainHashes[i], originalIndex+1)

			// Verificar si hay resultados mediante los scores
			scores := result.Scores
			if len(scores) == 0 {
				continue
			}

			var fileContentsCandidates []uint64
			var urlsCandidates []uint64
			purlHashMap := make(map[uint64]string)
			// Process results from scores
			for j := 0; j < len(scores); j++ {
				var lastPurl string
				var lastUrlHash uint64
				if scores[j] > scores[0]+2 {
					break
				}
				// Data filed processing
				if result.Fields != nil {
					for k, field := range result.Fields {
						var fieldName string
						// Get field name
						if namedField, ok := field.(interface{ Name() string }); ok {
							fieldName = namedField.Name()
						} else {
							fieldName = fmt.Sprintf("campo_%d", k)
						}

						// Field processing
						if binaryCol, ok := field.(*entity.ColumnBinaryVector); ok && binaryCol != nil {
							if dataMethod := binaryCol.Data; dataMethod != nil {
								data := dataMethod()
								if j < len(data) {
									if fieldName == "hfhDirs" {
										hash := binary.BigEndian.Uint64(data[j])
										fileContentsCandidates = append(fileContentsCandidates, hash)
									}
								}
							}
						} else if strCol, ok := field.(*entity.ColumnInt64); ok && strCol != nil {
							if dataMethod := strCol.Data; dataMethod != nil {
								data := dataMethod()
								if j < len(data) {
									if fieldName == "urlHash" {
										urlsCandidates = append(urlsCandidates, uint64(data[j]))
										lastUrlHash = uint64(data[j])
									}
								}
							}
						} else if strCol, ok := field.(*entity.ColumnVarChar); ok && strCol != nil {
							if dataMethod := strCol.Data; dataMethod != nil {
								data := dataMethod()
								if j < len(data) {
									if fieldName == "purl" {
										lastPurl = data[j]
									}
								}
							}
						}
					}
					purlHashMap[lastUrlHash] = lastPurl
				}
			}
			//select closest results based on content proximity
			distance, urls := rankClosest(fileContentsCandidates, urlsCandidates, currentSecHashes[i])

			var preferedUrls []uint64
			var preferedDistances []int

			var regularDistances []int
			var regularUrls []uint64

			for i, url := range urls {
				purl := purlHashMap[url]
				prefered := topPurls[purl]
				if prefered && (len(preferedDistances) == 0 || distance[i] < int(math.Ceil(float64(preferedDistances[0])*1.1))) {
					preferedUrls = append(preferedUrls, url)
					preferedDistances = append(preferedDistances, distance[i])
				} else if distance[i] <= distance[0]+4 || len(regularUrls) == 0 {
					regularUrls = append(regularUrls, url)
					regularDistances = append(regularDistances, distance[i])
				}
			}

			if len(preferedDistances) > 0 && preferedDistances[0] < int(math.Ceil(float64(distance[0])*1.1))+1 {
				matchedDistances[originalIndex] = preferedDistances[0]
				matchedUrls[originalIndex] = preferedUrls
			} else if len(preferedDistances) > 0 && preferedDistances[0] < int(math.Ceil((float64(distance[0])*1.2)))+1 {
				matchedDistances[originalIndex] = (regularDistances[0] + preferedDistances[0]) / 2
				matchedUrls[originalIndex] = append(regularUrls, preferedUrls...)
			} else {
				matchedDistances[originalIndex] = regularDistances[0]
				matchedUrls[originalIndex] = regularUrls
			}
		}
	}

	return matchedDistances, matchedUrls, nil
}

// Query the secondary hash table
func (db *MilvusDb) SecondarySearch(secHashes []uint64, maxDistance int) ([][]uint64, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Second)
	defer cancel()

	c, err := client.NewGrpcClient(ctx, db.address)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	err = c.UsingDatabase(ctx, db.databaseName)
	if err != nil {
		return nil, err
	}

	// Initialize result slices
	matchedHashNames := make([][]uint64, len(secHashes))

	searchResults, err := searchSimilarHashes(ctx, c, secHashes, db.TopMainResult, 5,
		secondaryColletionName, "hfhContents", []string{"hfhNames"}, nil)
	if err != nil {
		return nil, err
	}
	for i, result := range searchResults {
		scores := result.Scores
		db.s.Debugf("%x # %d matches found", secHashes[i], len(scores))
		if len(scores) == 0 {
			continue
		}

		var fileNamesCandidates []uint64
		minDistance := scores[0]
		if minDistance > float32(maxDistance) {
			continue
		}

		for j, s := range scores {
			if s > minDistance+2 {
				break
			}

			if result.Fields != nil {
				for k, field := range result.Fields {
					var fieldName string
					//get field name
					if namedField, ok := field.(interface{ Name() string }); ok {
						fieldName = namedField.Name()
					} else {
						fieldName = fmt.Sprintf("campo_%d", k)
					}

					//Field processing
					if binaryCol, ok := field.(*entity.ColumnBinaryVector); ok && binaryCol != nil {
						if dataMethod := binaryCol.Data; dataMethod != nil {
							data := dataMethod()
							if j < len(data) {
								if fieldName == "hfhNames" {
									hash := binary.BigEndian.Uint64(data[j])
									fileNamesCandidates = append(fileNamesCandidates, hash)
								}
							}
						}
					}
				}
			}
			matchedHashNames[i] = fileNamesCandidates
		}
	}

	return matchedHashNames, nil
}

// Get component information from url id
func (db *MilvusDb) GetComponent(urlKey uint64) ([]string, error) {
	outputFields := []string{"purl", "version", "release_date", "url"}
	//cast the url id to int64 (milvus doesn't support uint64)
	hashInt64 := int64(urlKey)
	expr := fmt.Sprintf("urlHash == %d", hashInt64)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c, err := client.NewGrpcClient(ctx, db.address)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	err = c.UsingDatabase(ctx, db.databaseName)
	if err != nil {
		return nil, err
	}

	queryResult, err := c.Query(ctx, mainColletionName, nil, expr, outputFields)
	if err != nil {
		return nil, err
	}

	if len(queryResult) == 0 || queryResult[0].Len() == 0 {
		return nil, nil
	}

	numResults := queryResult[0].Len()
	if numResults > 0 {
		for i := 0; i < numResults; i++ {
			result := make([]string, len(outputFields))
			// Process the results by field
			for i, fieldName := range outputFields {
				columnData := queryResult.GetColumn(fieldName)
				if columnData == nil {
					continue
				}

				switch data := columnData.(type) {
				case *entity.ColumnVarChar:
					result[i] = string(data.Data()[0])
				}
			}
			return result, nil
		}
	}
	return nil, nil
}

// Milvus proximity search wrapper
func searchSimilarHashes(ctx context.Context, c client.Client, searchValues []uint64, topK, nprobe int, collectionName string, fieldName string, outputFieldNames []string, partitions []string) ([]client.SearchResult, error) {

	// Convert each uint64 hash to a binary vector
	var vectors []entity.Vector
	for _, h := range searchValues {
		hashBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(hashBytes, h)
		var binaryVec entity.BinaryVector = hashBytes
		vectors = append(vectors, binaryVec)
	}

	// ANN confing. Bigger number, more presicition more time consuming.
	sp, err := entity.NewIndexBinFlatSearchParam(nprobe)
	if err != nil {
		return nil, fmt.Errorf("failed to setup searching parameter %v", err)
	}

	// Intentar la búsqueda con la firma correcta para tu SDK
	searchResults, err := c.Search(
		ctx,              // context
		collectionName,   // collection name
		partitions,       // partitions
		"",               // expresión filter
		outputFieldNames, // output fields
		vectors,          // vectores de búsqueda (BinaryVector)
		fieldName,        // nombre del campo vector (string, no []string)
		entity.HAMMING,   // métrica (Hamming ideal para vectores binarios)
		topK,             // top K resultados
		sp,               // search params
	)

	return searchResults, err
}

func rankClosest(candidates []uint64, urls []uint64, contentsHash uint64) ([]int, []uint64) {
	var distances []int

	//minFilesContentdistance := minDistance
	//use the filecontents hash to select the best results
	for _, key := range candidates {
		filesContentdistance := hammingDistance(contentsHash, key)
		distances = append(distances, filesContentdistance)
	}

	// Verify that all three lists have the same length
	if len(candidates) != len(urls) || len(candidates) != len(distances) {
		panic("All three lists must have the same length")
	}

	// Create a slice of indices
	indices := make([]int, len(distances))
	for i := range indices {
		indices[i] = i
	}

	// Sort the indices based on the values in distances
	sort.Slice(indices, func(i, j int) bool {
		return distances[indices[i]] < distances[indices[j]]
	})

	sortedUrls := make([]uint64, len(urls))
	sortedDistances := make([]int, len(distances))

	// Rearrange according to the sorted indices
	for i, idx := range indices {
		sortedUrls[i] = urls[idx]
		sortedDistances[i] = distances[idx]
	}

	return sortedDistances, sortedUrls
}

func hammingDistance(x, y uint64) int {
	xor := x ^ y
	return bits.OnesCount64(xor)
}
