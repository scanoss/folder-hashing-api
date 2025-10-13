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

package service

import (
	"context"
	"sort"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/folder-hashing-api/internal/repository"
	"github.com/scanoss/folder-hashing-api/internal/validation"
)

// ScanServiceImpl implements the ScanService interface.
type ScanServiceImpl struct {
	repo repository.ScanRepository
}

// NewScanService creates a new scan service instance.
func NewScanService(repo repository.ScanRepository) ScanService {
	return &ScanServiceImpl{
		repo: repo,
	}
}

// ScanFolder performs a folder hash scan.
func (s *ScanServiceImpl) ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error) {
	logger := ctxzap.Extract(ctx).Sugar()

	if err := validation.ValidateStruct(req); err != nil {
		return nil, err
	}

	results, err := s.scanNode(ctx, req.Root, req.RankThreshold, req.RecursiveThreshold, req.MinAcceptedScore, true)
	if err != nil {
		return nil, err
	}

	// Deduplicate components across folders, keeping only the highest scoring instance
	results = s.deduplicateComponents(results)

	response := &entities.ScanResponse{
		Results: results,
	}

	logger.Info("Scan completed: found %d results", len(results))

	return response, nil
}

func (s *ScanServiceImpl) processComponentGroups(componentGroups []entities.ComponentGroup, path string, minAcceptedScore float32) []*entities.ScanResult {
	if len(componentGroups) == 0 {
		return []*entities.ScanResult{}
	}

	var results []*entities.ScanResult
	var filteredGroups []*entities.ComponentGroup

	// Filter component groups based on minimum accepted score
	for _, group := range componentGroups {
		// Filter versions within the group
		var filteredVersions []entities.Version
		for _, version := range group.Versions {
			if version.Score > minAcceptedScore {
				filteredVersions = append(filteredVersions, version)
			}
		}

		// Only include the group if it has at least one version above the threshold
		if len(filteredVersions) > 0 {
			groupCopy := group
			groupCopy.Versions = filteredVersions
			filteredGroups = append(filteredGroups, &groupCopy)
		}
	}

	// Only create a result if we have filtered groups
	if len(filteredGroups) > 0 {
		result := &entities.ScanResult{
			PathID:          path,
			ComponentGroups: filteredGroups,
		}
		results = append(results, result)
	}

	return results
}

func (s *ScanServiceImpl) scanNode(ctx context.Context, node *entities.FolderNode, rankThreshold int, recursiveThreshold, minAcceptedScore float32, isRoot bool) ([]*entities.ScanResult, error) {
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
	shouldCheckThreshold := !isRoot || len(node.Children) == 0

	// Check if any component group has a version with score >= recursiveThreshold
	if shouldCheckThreshold && recursiveThreshold > 0 && s.hasHighScoreMatch(componentGroups, recursiveThreshold) {
		logger.Infof("Found high score match (>= %f) for node %s, stopping search", recursiveThreshold, node.PathID)
		results := s.processComponentGroups(componentGroups, node.PathID, minAcceptedScore)
		return results, nil
	}

	results := s.processComponentGroups(componentGroups, node.PathID, minAcceptedScore)

	if len(node.Children) > 0 {
		for _, child := range node.Children {
			childResults, err := s.scanNode(ctx, child, rankThreshold, recursiveThreshold, minAcceptedScore, false)
			if err != nil {
				return nil, err
			}
			results = append(results, childResults...)
		}
	}

	return results, nil
}

// hasHighScoreMatch checks if any component group has a version with score >= threshold.
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

// deduplicateComponents removes duplicate components across folders, keeping only the highest scoring instance.
//
//nolint:gocognit // Deduplication algorithm complexity is acceptable
func (s *ScanServiceImpl) deduplicateComponents(results []*entities.ScanResult) []*entities.ScanResult {
	if len(results) == 0 {
		return results
	}

	// Map to track the best component instance: PURL -> (pathID, componentGroup, maxScore)
	type componentInfo struct {
		pathID    string
		component *entities.ComponentGroup
		maxScore  float32
	}
	bestComponents := make(map[string]*componentInfo)

	// Find the highest scoring instance of each component
	for _, result := range results {
		for _, group := range result.ComponentGroups {
			// Find the maximum score for this component group
			var maxScore float32
			for _, version := range group.Versions {
				if version.Score > maxScore {
					maxScore = version.Score
				}
			}

			// Check if we've seen this component before
			if existing, exists := bestComponents[group.PURL]; exists {
				// Keep the one with higher score
				if maxScore > existing.maxScore {
					bestComponents[group.PURL] = &componentInfo{
						pathID:    result.PathID,
						component: group,
						maxScore:  maxScore,
					}
				}
			} else {
				// First time seeing this component
				bestComponents[group.PURL] = &componentInfo{
					pathID:    result.PathID,
					component: group,
					maxScore:  maxScore,
				}
			}
		}
	}

	// Rebuild results with deduplicated components
	pathToComponents := make(map[string][]*entities.ComponentGroup)
	for _, info := range bestComponents {
		pathToComponents[info.pathID] = append(pathToComponents[info.pathID], info.component)
	}

	// Create new result set
	var deduplicatedResults []*entities.ScanResult
	for pathID, components := range pathToComponents {
		if len(components) > 0 {
			deduplicatedResults = append(deduplicatedResults, &entities.ScanResult{
				PathID:          pathID,
				ComponentGroups: components,
			})
		}
	}

	sort.Slice(deduplicatedResults, func(i, j int) bool {
		return deduplicatedResults[i].PathID < deduplicatedResults[j].PathID
	})

	return deduplicatedResults
}
