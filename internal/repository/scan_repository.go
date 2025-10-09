// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2024 SCANOSS.COM
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 2 of the License, or
 * (at your option) any later version.
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

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
