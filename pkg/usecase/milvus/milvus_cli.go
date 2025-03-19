package milvus

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math/bits"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

var (
	mainColletionName    = "main"
	OutputFieldNamesMain = []string{"hfhNames", "hfhContents", "url"} // Campos a devolver
	OutputFieldNamesSec  = []string{"hfhContents", "hfhNames"}        // Campos a devolver
	host                 = "localhost"
	port                 = "19530"
)

type MilvusDb struct {
	client        *client.Client
	ctx           *context.Context
	TopMainResult int
}

func NewMilvusDb() *MilvusDb {
	// Conectar a Milvus
	milvusAddress := fmt.Sprintf("%s:%s", host, port)
	log.Printf("Conectando a Milvus en %s...", milvusAddress)
	ctx, _ := context.WithTimeout(context.Background(), 600*time.Second)
	//defer cancel()

	c, err := client.NewGrpcClient(ctx, milvusAddress)
	if err != nil {
		log.Fatalf("Error al conectar a Milvus: %v", err)
	}
	//defer c.Close()
	log.Println("Conexión establecida correctamente")

	// Verificar que la colección existe
	has, err := c.HasCollection(ctx, mainColletionName)
	if err != nil {
		log.Fatalf("Error al verificar colección: %v", err)
	}
	if !has {
		log.Fatalf("Error: La colección %s no existe", mainColletionName)
	}
	return &MilvusDb{client: &c, ctx: &ctx, TopMainResult: 10000}
}

func (db *MilvusDb) Mainsearch(mainHashes []uint64, secHashes []uint64) ([]int, [][]string, error) {

	searchResults, err := searchSimilarHashes(*db.ctx, *db.client, mainHashes, db.TopMainResult, "main", "hfhNames", []string{"hfhContents", "url"}, nil)
	matchedUrls := make([][]string, len(mainHashes))
	matchedDistances := make([]int, len(mainHashes))
	if err != nil {
		return nil, nil, err
	}

	for i, result := range searchResults {
		fmt.Printf("Resultados para consulta #%d:\n", i+1)

		// Verificar si hay resultados mediante los scores
		scores := result.Scores
		if len(scores) == 0 {
			fmt.Println("  No se encontraron resultados")
			continue
		}

		var fileContentsCandidates []uint64
		var urlsCandidates []string

		// Procesar cada resultado basándonos en el índice de los scores
		for j := 0; j < len(scores); j++ {
			fmt.Printf("  Resultado #%d: Distancia: %.2f\n", j+1, scores[j])

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
								fmt.Printf("    %s: %x\n", fieldName, data[j])
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
								fmt.Printf("    %s: %s\n", fieldName, data[j])
								if fieldName == "url" {
									urlsCandidates = append(urlsCandidates, string(data[j]))
								}
							}
						}
					}
				}
			}
		}

		distance, urls := selectClosest(fileContentsCandidates, urlsCandidates, secHashes[i])
		matchedDistances[i] = distance
		matchedUrls[i] = urls
	}

	return matchedDistances, matchedUrls, nil
}

// searchSimilarHashes busca hashes similares en Milvus usando entity.BinaryVector
func searchSimilarHashes(ctx context.Context, c client.Client, searchValues []uint64, topK int, collectionName string, fieldName string, outputFieldNames []string, partitions []string) ([]client.SearchResult, error) {
	// Cargar la colección
	err := c.LoadCollection(ctx, collectionName, true)
	if err != nil {
		return nil, fmt.Errorf("error al cargar colección: %v", err)
	}
	defer func() {
		releaseErr := c.ReleaseCollection(ctx, collectionName)
		if releaseErr != nil {
			log.Printf("Advertencia: error al liberar colección: %v", releaseErr)
		}
	}()

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

func selectClosest(candidates []uint64, urls []string, contentsHash uint64) (int, []string) {
	var bestMatches []uint64
	var bestUrlsKey []string

	minFilesContentdistance := 30
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
