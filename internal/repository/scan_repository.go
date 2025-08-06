package repository

import (
	"context"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

type CollectionStats struct {
	Name          string
	Status        string
	PointsCount   uint64
	SegmentsCount uint64
}

type ScanRepository interface {
	// SearchByHashes performs a search using directory, name, and content hashes
	SearchByHashes(ctx context.Context, dirHash, nameHash, contentHash string, langExt entities.LanguageExtensions, rankThreshold int) ([]entities.ComponentGroup, error)

	// GetCollectionStats returns statistics for a given collection
	GetCollectionStats(ctx context.Context, collectionName string) (*CollectionStats, error)

	// CollectionExists checks if a collection exists
	CollectionExists(ctx context.Context, collectionName string) (bool, error)

	// GetAllSupportedCollections returns all supported collection names
	GetAllSupportedCollections() []string
}
