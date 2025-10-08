// Package repository provides data access implementations for the HFH service.
package repository

import (
	"context"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// CollectionStats contains statistics for a Qdrant collection.
type CollectionStats struct {
	Name          string
	Status        string
	PointsCount   uint64
	SegmentsCount uint64
}

// ScanRepository defines the interface for scan data access operations.
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
