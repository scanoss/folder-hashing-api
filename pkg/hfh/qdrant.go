package hfh

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

const (
	VectorDim = 192 // Three 64-bit hashes concatenated (3 * 64 = 192)
)

// QdrantConfig holds Qdrant connection configuration
type QdrantConfig struct {
	Host           string
	Port           int
	CollectionName string
}

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	Score       float32
	ID          uint64
	CombinedHash string
	Vendor      string
	Component   string
	Version     string
	URL         string
	Metadata    map[string]interface{}
}


// SearchSimilarProjects searches for similar projects in Qdrant based on three separate hashes
func SearchSimilarProjects(config QdrantConfig, dirHash, nameHash, contentHash uint64, topK uint64) ([]SearchResult, error) {
	// Create Qdrant client
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("error connecting to Qdrant: %v", err)
	}
	defer client.Close()

	// Check if collection exists
	ctx := context.Background()
	exists, err := client.CollectionExists(ctx, config.CollectionName)
	if err != nil {
		return nil, fmt.Errorf("error checking collection existence: %v", err)
	}
	if !exists {
		return nil, fmt.Errorf("collection '%s' does not exist", config.CollectionName)
	}

	// Convert three hashes to concatenated vector
	queryVector := HashesToVector(dirHash, nameHash, contentHash)

	// Perform similarity search using Query method
	queryReq := &qdrant.QueryPoints{
		CollectionName: config.CollectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(uint64(topK)),
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false), // We don't need vectors in response
	}

	searchResp, err := client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("error performing search: %v", err)
	}

	// Convert response to SearchResult structs
	var results []SearchResult
	for _, point := range searchResp {
		result := SearchResult{
			Score: point.Score,
			ID:    point.Id.GetNum(),
			Metadata: make(map[string]interface{}),
		}

		// Extract payload fields if they exist
		if point.Payload != nil {
			if val, exists := point.Payload["combined_hash"]; exists {
				result.CombinedHash = val.GetStringValue()
			}
			if val, exists := point.Payload["vendor"]; exists {
				result.Vendor = val.GetStringValue()
			}
			if val, exists := point.Payload["component"]; exists {
				result.Component = val.GetStringValue()
			}
			if val, exists := point.Payload["version"]; exists {
				result.Version = val.GetStringValue()
			}
			if val, exists := point.Payload["url"]; exists {
				result.URL = val.GetStringValue()
			}

			// Store all payload for access to other fields
			for key, value := range point.Payload {
				switch {
				case value.GetStringValue() != "":
					result.Metadata[key] = value.GetStringValue()
				case value.GetIntegerValue() != 0:
					result.Metadata[key] = value.GetIntegerValue()
				case value.GetDoubleValue() != 0:
					result.Metadata[key] = value.GetDoubleValue()
				case value.GetBoolValue():
					result.Metadata[key] = value.GetBoolValue()
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}
