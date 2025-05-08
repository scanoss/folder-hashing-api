package milvus

import (
	"reflect"
	"testing"
)

func TestMilvusMainQueryExactMatch(t *testing.T) {
	db, err := NewMilvusDb("", "", "test")
	if err != nil {
		t.Error(err)
	}

	//test for exact match
	namesHash := []uint64{0x9db5ef192baf3b4e}
	contentsHash := []uint64{0x5cb916d72230f456}
	distances, urls, err := db.Mainsearch(namesHash, contentsHash, 10, nil)
	if err != nil {
		t.Error(err)
	}

	t.Log("matched distances:", distances)
	t.Logf("Matched urls: %x", urls)
	expectedUrls := [][]uint64{
		{0x10a1de5e02e61411, 0x3c130c43938a5447},
	}

	if !reflect.DeepEqual(urls, expectedUrls) || distances[0] > 0 {
		t.Errorf("Expected: %#x\n result : %#x", expectedUrls, urls)
	}
}

func TestMilvusMainQueryParcialMatch(t *testing.T) {
	db, err := NewMilvusDb("", "", "test")
	if err != nil {
		t.Error(err)
	}

	//test for exact match
	namesHash := []uint64{0x9db5ff192baf3c4e}
	contentsHash := []uint64{0x5cc916d72130f456}
	distances, urls, err := db.Mainsearch(namesHash, contentsHash, 10, nil)
	if err != nil {
		t.Error(err)
	}

	t.Log("matched distances:", distances)
	t.Logf("Matched urls: %x", urls)
	expectedUrls := [][]uint64{
		{0x10a1de5e02e61411, 0x3c130c43938a5447, 0x8357efaedbcc5cb7},
	}

	if !reflect.DeepEqual(urls, expectedUrls) {
		t.Errorf("Expected: %#x\n result : %#x", expectedUrls, urls)
	}
}

func TestMilvusSecondarySearch(t *testing.T) {
	db, err := NewMilvusDb("", "", "test")
	if err != nil {
		t.Error(err)
	}

	contentsHash := []uint64{0x4fff9fbafff17df7, 0x01201b887afc8ad6}
	results, err := db.SecondarySearch(contentsHash, 10)
	if err != nil {
		t.Error(err)
	}

	expectedResults := [][]uint64{
		{0x9db5ef192baf3b4e, 0xbfb5ef112b0f7b4e}, {0xaf63bd4c8601b7ab}}

	if !reflect.DeepEqual(results, expectedResults) {
		t.Errorf("Expected: %#x\n result : %#x", expectedResults, results)
	}
}
