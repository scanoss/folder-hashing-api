package service

import (
	"context"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// ScanService defines the interface for scan-related business operations
type ScanService interface {
	// ScanFolder performs a folder hash scan
	ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error)
}
