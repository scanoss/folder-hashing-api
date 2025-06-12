package service

import (
	"context"
	"log"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	domainErrors "github.com/scanoss/folder-hashing-api/internal/domain/errors"
	"github.com/scanoss/folder-hashing-api/internal/repository"
)

// scanService implements the ScanService interface
type scanService struct {
	scanRepo repository.ScanRepository
}

// NewScanService creates a new scan service
func NewScanService(scanRepo repository.ScanRepository) ScanService {
	return &scanService{
		scanRepo: scanRepo,
	}
}

// ScanFolder performs a folder hash scan
func (s *scanService) ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error) {
	// Validate input
	if req == nil {
		return nil, domainErrors.NewInvalidRequestError("request cannot be nil")
	}
	if req.Root == nil {
		return nil, domainErrors.NewInvalidRequestError("root folder node cannot be nil")
	}

	log.Printf("Processing scan request for path: %s", req.Root.PathID)

	// Extract hashes from root node
	dirHash := req.Root.SimHashDirNames
	nameHash := req.Root.SimHashNames
	contentHash := req.Root.SimHashContent
	langExt := req.Root.LangExtensions

	// Validate hashes
	if dirHash == "" || nameHash == "" || contentHash == "" {
		return nil, domainErrors.NewInvalidRequestError("directory, name, and content hashes are required")
	}

	// Perform search using repository
	componentGroups, err := s.scanRepo.SearchByHashes(ctx, dirHash, nameHash, contentHash, langExt, 100)
	if err != nil {
		return nil, err
	}

	// Apply business logic
	results := s.processComponentGroups(componentGroups, req)

	// Create response
	response := &entities.ScanResponse{
		Results: results,
		Status: &entities.StatusResponse{
			Code:    200,
			Message: "success",
		},
	}

	log.Printf("Scan completed: found %d results", len(results))
	return response, nil
}

// processComponentGroups applies business logic to component groups
func (s *scanService) processComponentGroups(componentGroups []entities.ComponentGroup, req *entities.ScanRequest) []*entities.ScanResult {
	if len(componentGroups) == 0 {
		return []*entities.ScanResult{}
	}

	// Apply threshold filtering if specified
	if req.Threshold > 0 {
		componentGroups = s.applyThresholdFilter(componentGroups, req.Threshold)
	}

	// Apply best match logic if requested
	if req.BestMatch {
		componentGroups = s.applyBestMatchFilter(componentGroups)
	}

	// Convert to scan results
	var results []*entities.ScanResult

	// For now, we create a single result for the root path with all component groups
	// This can be extended to support hierarchical scanning later
	if len(componentGroups) > 0 {
		result := &entities.ScanResult{
			PathID:          req.Root.PathID,
			ComponentGroups: make([]*entities.ComponentGroup, len(componentGroups)),
			Probability:     s.calculateOverallProbability(componentGroups),
			Stage:           1, // Basic stage for now
		}

		// Copy component groups to result
		for i, group := range componentGroups {
			groupCopy := group // Create a copy
			result.ComponentGroups[i] = &groupCopy
		}

		results = append(results, result)
	}

	return results
}

// applyThresholdFilter filters component groups based on threshold
func (s *scanService) applyThresholdFilter(componentGroups []entities.ComponentGroup, threshold int32) []entities.ComponentGroup {
	var filtered []entities.ComponentGroup

	// Convert threshold to score (assuming threshold is a percentage, e.g., 60 = 0.6)
	thresholdScore := float32(threshold) / 100.0

	for _, group := range componentGroups {
		if group.BestMatch.Score >= thresholdScore {
			filtered = append(filtered, group)
		}
	}

	log.Printf("Threshold filtering: %d groups passed threshold of %d (%.2f)", len(filtered), threshold, thresholdScore)
	return filtered
}

// applyBestMatchFilter returns only the best matching component group
func (s *scanService) applyBestMatchFilter(componentGroups []entities.ComponentGroup) []entities.ComponentGroup {
	if len(componentGroups) == 0 {
		return componentGroups
	}

	// Return only the first group (which should be the best due to sorting in repository)
	bestMatch := componentGroups[0]
	log.Printf("Best match filtering: selected %s with score %.4f", bestMatch.Component, bestMatch.BestMatch.Score)
	return []entities.ComponentGroup{bestMatch}
}

// calculateOverallProbability calculates the overall probability from component groups
func (s *scanService) calculateOverallProbability(componentGroups []entities.ComponentGroup) float32 {
	if len(componentGroups) == 0 {
		return 0.0
	}

	// Use the best match score as the overall probability
	// This could be enhanced with more sophisticated algorithms
	return componentGroups[0].BestMatch.Score
}
