package service

import (
	"context"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

type ScanService interface {
	// ScanFolder performs a folder hashing scan
	ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error)
}
