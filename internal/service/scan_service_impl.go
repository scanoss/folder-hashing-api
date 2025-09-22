package service

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/folder-hashing-api/internal/repository"
	"github.com/scanoss/folder-hashing-api/internal/validation"
)

type ScanServiceImpl struct {
	repo repository.ScanRepository
}

func NewScanService(repo repository.ScanRepository) ScanService {
	return &ScanServiceImpl{
		repo: repo,
	}
}

// ScanFolder performs a folder hash scan
func (s *ScanServiceImpl) ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error) {
	logger := ctxzap.Extract(ctx).Sugar()

	if err := validation.ValidateStruct(req); err != nil {
		return nil, err
	}

	results, err := s.scanNode(ctx, req.Root, req.RankThreshold, req.RecursiveThreshold, true)
	if err != nil {
		return nil, err
	}

	response := &entities.ScanResponse{
		Results: results,
	}

	logger.Info("Scan completed: found %d results", len(results))

	return response, nil
}

func (s *ScanServiceImpl) processComponentGroups(componentGroups []entities.ComponentGroup, path string) []*entities.ScanResult {
	if len(componentGroups) == 0 {
		return []*entities.ScanResult{}
	}

	var results []*entities.ScanResult

	result := &entities.ScanResult{
		PathID:          path,
		ComponentGroups: make([]*entities.ComponentGroup, len(componentGroups)),
	}

	for i, group := range componentGroups {
		groupCopy := group
		result.ComponentGroups[i] = &groupCopy
	}

	results = append(results, result)

	return results
}

func (s *ScanServiceImpl) scanNode(ctx context.Context, node *entities.FolderNode, rankThreshold int, recursiveThreshold float32, isRoot bool) ([]*entities.ScanResult, error) {
	logger := ctxzap.Extract(ctx).Sugar()

	if node.SimHashDirNames == "" && node.SimHashNames == "" && node.SimHashContent == "" {
		logger.Debugf("Skipping node %s - all hashes are empty", node.PathID)
		return []*entities.ScanResult{}, nil
	}

	logger.Debugf("Searching node %s with hashes - dirs: %s, names: %s, content: %s", node.PathID, node.SimHashDirNames, node.SimHashNames, node.SimHashContent)

	componentGroups, err := s.repo.SearchByHashes(ctx, node.SimHashDirNames, node.SimHashNames, node.SimHashContent, node.LangExtensions, rankThreshold)
	if err != nil {
		return nil, err
	}

	logger.Debugf("SearchByHashes returned %d component groups for node %s", len(componentGroups), node.PathID)

	// Skip recursive threshold check for root node when depth is enabled (has children)
	shouldCheckThreshold := !(isRoot && len(node.Children) > 0)

	// Check if any component group has a version with score >= recursiveThreshold
	if shouldCheckThreshold && recursiveThreshold > 0 && s.hasHighScoreMatch(componentGroups, recursiveThreshold) {
		logger.Infof("Found high score match (>= %f) for node %s, stopping search", recursiveThreshold, node.PathID)
		results := s.processComponentGroups(componentGroups, node.PathID)
		return results, nil
	}

	results := s.processComponentGroups(componentGroups, node.PathID)

	if len(node.Children) > 0 {
		for _, child := range node.Children {
			childResults, err := s.scanNode(ctx, child, rankThreshold, recursiveThreshold, false)
			if err != nil {
				return nil, err
			}
			results = append(results, childResults...)
		}
	}

	return results, nil
}

// hasHighScoreMatch checks if any component group has a version with score >= threshold
func (s *ScanServiceImpl) hasHighScoreMatch(componentGroups []entities.ComponentGroup, threshold float32) bool {
	for _, group := range componentGroups {
		for _, version := range group.Versions {
			if version.Score >= threshold {
				return true
			}
		}
	}
	return false
}
