package milvus

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math/bits"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"go.uber.org/zap"
)

var (
	mainColletionName      = "main"
	secondaryColletionName = "secondary"
	defaultTopResults      = 10000

	OutputFieldNamesMain = []string{"hfhNames", "hfhContents", "url"} // Campos a devolver
	OutputFieldNamesSec  = []string{"hfhContents", "hfhNames"}        // Campos a devolver
	defaultHost          = "localhost"
	defaultPort          = "19530"
)

type MilvusDb struct {
	address       string
	s             *zap.SugaredLogger
	TopMainResult int
}

func NewMilvusDb(host string, port string) (*MilvusDb, error) {

	if host == "" {
		host = defaultHost
	}

	if port == "" {
		port = defaultPort
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
	return &MilvusDb{s: s, address: milvusAddress, TopMainResult: defaultTopResults}, nil
}

func (db *MilvusDb) Mainsearch(mainHashes []uint64, secHashes []uint64, minDistance int) ([]int, [][]string, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c, err := client.NewGrpcClient(ctx, db.address)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close()
	// Make sure input slices have the same length
	if len(mainHashes) != len(secHashes) {
		return nil, nil, fmt.Errorf("mainHashes and secHashes must have the same length")
	}

	// Initialize result slices
	matchedDistances := make([]int, len(mainHashes))
	matchedUrls := make([][]string, len(mainHashes))

	// Default all distances to 999 (indicating no match)
	for i := range matchedDistances {
		matchedDistances[i] = 999
	}

	// Process in blocks of 100
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
		searchResults, err := searchSimilarHashes(ctx, c, currentMainHashes, db.TopMainResult,
			mainColletionName, "hfhNames", []string{"hfhContents", "url"}, nil)
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

			/*if len(scores) > 5000 && scores[0]+5 < scores[len(scores)/2] {
				searchResultsb, err := searchSimilarHashes(ctx, c, []uint64{secHashes[i]}, db.TopMainResult,
					mainColletionName, "hfhContents", []string{"hfhContents", "url"}, nil)

				if err == nil && len(searchResultsb[0].Scores) > 0 {
					result = searchResultsb[0]
				}
			}*/

			var fileContentsCandidates []uint64
			var urlsCandidates []string

			// Procesar cada resultado basándonos en el índice de los scores
			for j := 0; j < len(scores); j++ {
				// Procesar campos adicionales
				if result.Fields != nil {
					for k, field := range result.Fields {
						var fieldName string
						// Obtener el nombre del campo
						if namedField, ok := field.(interface{ Name() string }); ok {
							fieldName = namedField.Name()
						} else {
							fieldName = fmt.Sprintf("campo_%d", k)
						}

						// Extraer valor según el tipo
						if binaryCol, ok := field.(*entity.ColumnBinaryVector); ok && binaryCol != nil {
							if dataMethod := binaryCol.Data; dataMethod != nil {
								data := dataMethod()
								if j < len(data) {
									if fieldName == "hfhContents" {
										hash := binary.BigEndian.Uint64(data[j])
										fileContentsCandidates = append(fileContentsCandidates, hash)
									}
								}
							}
						} else if strCol, ok := field.(*entity.ColumnVarChar); ok && strCol != nil {
							if dataMethod := strCol.Data; dataMethod != nil {
								data := dataMethod()
								if j < len(data) {
									if fieldName == "url" {
										urlsCandidates = append(urlsCandidates, string(data[j]))
									}
								}
							}
						}
					}
				}
			}

			distance, urls := selectClosest(fileContentsCandidates, urlsCandidates, currentSecHashes[i], minDistance*3/4)
			matchedDistances[originalIndex] = distance
			matchedUrls[originalIndex] = urls
		}
	}

	return matchedDistances, matchedUrls, nil
}

func (db *MilvusDb) SecondarySearch(secHashes []uint64, maxDistance int) ([][]uint64, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c, err := client.NewGrpcClient(ctx, db.address)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// Initialize result slices
	matchedHashNames := make([][]uint64, len(secHashes))

	searchResults, err := searchSimilarHashes(ctx, c, secHashes, db.TopMainResult,
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
					// Obtener el nombre del campo
					if namedField, ok := field.(interface{ Name() string }); ok {
						fieldName = namedField.Name()
					} else {
						fieldName = fmt.Sprintf("campo_%d", k)
					}

					// Extraer valor según el tipo
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

// searchSimilarHashes busca hashes similares en Milvus usando entity.BinaryVector
func searchSimilarHashes(ctx context.Context, c client.Client, searchValues []uint64, topK int, collectionName string, fieldName string, outputFieldNames []string, partitions []string) ([]client.SearchResult, error) {
	// Cargar la colección
	/*err := c.LoadCollection(ctx, collectionName, false)
	if err != nil {
		return nil, fmt.Errorf("error al cargar colección: %v", err)
	}
	defer func() {
		releaseErr := c.ReleaseCollection(ctx, collectionName)
		if releaseErr != nil {
			log.Printf("Advertencia: error al liberar colección: %v", releaseErr)
		}
	}()*/

	// Crear un slice de vectores para la búsqueda
	var vectors []entity.Vector
	for _, h := range searchValues {
		// Convertir el hash uint64 a un vector binario (formato de 8 bytes)
		hashBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(hashBytes, h)
		var binaryVec entity.BinaryVector = hashBytes
		vectors = append(vectors, binaryVec)
	}

	// Configurar parámetros de búsqueda para ANN
	sp, err := entity.NewIndexBinFlatSearchParam(10)
	if err != nil {
		return nil, fmt.Errorf("error al crear parámetros de búsqueda: %v", err)
	}

	// Ejecutar la búsqueda con entity.BinaryVector
	log.Println("Ejecutando búsqueda con BinaryVector...")

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

func selectClosest(candidates []uint64, urls []string, contentsHash uint64, minDistance int) (int, []string) {
	var bestMatches []uint64
	var bestUrlsKey []string

	minFilesContentdistance := minDistance
	//use the filecontents hash to select the best results
	for i, key := range candidates {

		filesContentdistance := hammingDistance(contentsHash, key)
		//if the match is not exact with accept a range of valid distances
		if (filesContentdistance < minFilesContentdistance-8) || (minFilesContentdistance > 0 && filesContentdistance == 0) {
			bestMatches = []uint64{key}
			minFilesContentdistance = filesContentdistance
			bestUrlsKey = []string{urls[i]}
		} else if filesContentdistance <= minFilesContentdistance {
			bestMatches = append(bestMatches, key)
			bestUrlsKey = append(bestUrlsKey, urls[i])
		}
	}

	return minFilesContentdistance, bestUrlsKey
}

func hammingDistance(x, y uint64) int {
	xor := x ^ y
	return bits.OnesCount64(xor)
}

func headCalc(simHash uint64) byte {
	var sum int
	for i := 0; i < 8; i++ {
		b := byte((simHash >> (i * 8)) & 0xFF)
		sum += int(b) * 2
	}
	return byte(sum >> 4 & 0xFF)
}
