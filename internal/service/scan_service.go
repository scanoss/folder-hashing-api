// Package service implements business logic for the HFH service.
package service

import (
	"context"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// ScanService defines the interface for folder hashing scan operations.
type ScanService interface {
	// ScanFolder performs a folder hashing scan
	ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error)
}
