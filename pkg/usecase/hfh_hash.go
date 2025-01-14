package usecase

import (
	"log"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mfonda/simhash"
)

type HFHhash struct {
	NameHash    uint64
	ContentHash uint64
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

func HashCalc(node *DirectoryNode) HFHhash {
	processedHashes := make(map[string]bool)
	var fileHashesList [][]byte
	var selectedNames []string

	for _, file := range node.Files {
		if _, processed := processedHashes[file.KeyStr]; processed {
			continue
		}
		if file.Actions.StoreInFile {
			processedHashes[file.KeyStr] = true
			selectedNames = append(selectedNames, filepath.Base(file.Path))
			fileHashesList = append(fileHashesList, file.Key)
		}
	}

	sort.Strings(selectedNames)
	concatenatedNames := strings.Join(selectedNames, "")

	/* Calc Files name simhash */
	FilesNameSimhash := simhash.Simhash(simhash.NewWordFeatureSet([]byte(concatenatedNames)))
	/* Calc Files content simhash */
	FilesContentSimhash := simhash.Fingerprint(simhash.VectorizeBytes(fileHashesList))
	log.Printf("%s contents simhash: %x\n", node.Path, FilesContentSimhash)

	/* Calc hash head to group close hashes by sector */
	head := headCalc(FilesNameSimhash)
	//log.Printf("Main hash head: %02x\n", head)

	/*Overwrite the MS byte with the head to keep the hash size total */
	FilesNameSimhash = (FilesNameSimhash & 0x00FFFFFFFFFFFFFF) | (uint64(head) << 56)
	log.Printf("%s: names simhash: %x\n", node.Path, FilesNameSimhash)

	return HFHhash{
		NameHash:    FilesNameSimhash,
		ContentHash: FilesContentSimhash,
	}
}
