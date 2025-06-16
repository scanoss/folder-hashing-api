package service

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/folder-hashing-api/internal/repository"
	"github.com/scanoss/folder-hashing-api/internal/validation"
)

const (
	defaultTopK = 100
)

type ScanServiceImpl struct {
	scanRepo repository.ScanRepository
}

func NewScanService(scanRepo repository.ScanRepository) ScanService {
	return &ScanServiceImpl{
		scanRepo: scanRepo,
	}
}

// ScanFolder performs a folder hash scan
func (s *ScanServiceImpl) ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error) {
	logger := ctxzap.Extract(ctx).Sugar()

	topK := uint64(defaultTopK)
	if req.QueryLimit > 0 {
		topK = uint64(req.QueryLimit)
	}

	if err := validation.ValidateStruct(req); err != nil {
		return nil, err
	}

	dirHash := req.Root.SimHashDirNames
	nameHash := req.Root.SimHashNames
	contentHash := req.Root.SimHashContent
	langExt := req.Root.LangExtensions

	componentGroups, err := s.scanRepo.SearchByHashes(ctx, dirHash, nameHash, contentHash, langExt, topK)
	if err != nil {
		return nil, err
	}

	results := s.processComponentGroups(componentGroups, req)

	response := &entities.ScanResponse{
		Results: results,
		Status: &entities.StatusResponse{
			Code:    200,
			Message: "success",
		},
	}

	logger.Info("Scan completed: found %d results", len(results))

	return response, nil
}

func (s *ScanServiceImpl) processComponentGroups(componentGroups []entities.ComponentGroup, req *entities.ScanRequest) []*entities.ScanResult {
	if len(componentGroups) == 0 {
		return []*entities.ScanResult{}
	}

	var results []*entities.ScanResult

	if len(componentGroups) > 0 {
		result := &entities.ScanResult{
			PathID:          req.Root.PathID,
			ComponentGroups: make([]*entities.ComponentGroup, len(componentGroups)),
		}

		for i, group := range componentGroups {
			groupCopy := group
			result.ComponentGroups[i] = &groupCopy
		}

		results = append(results, result)
	}

	return results
}
