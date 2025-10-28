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
	"testing"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

func TestSortByBestComponentOrder(t *testing.T) {
	tests := []struct {
		name     string
		input    []*entities.ScanResult
		expected []string // Expected PathIDs in order
	}{
		{
			name:     "Empty slice",
			input:    []*entities.ScanResult{},
			expected: []string{},
		},
		{
			name: "Single result",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 5},
					},
				},
			},
			expected: []string{"/path/one"},
		},
		{
			name: "Two results - ascending order",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 1},
					},
				},
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 2},
					},
				},
			},
			expected: []string{"/path/one", "/path/two"},
		},
		{
			name: "Two results - descending order (needs sorting)",
			input: []*entities.ScanResult{
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 5},
					},
				},
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 2},
					},
				},
			},
			expected: []string{"/path/one", "/path/two"},
		},
		{
			name: "Multiple results with various orders",
			input: []*entities.ScanResult{
				{
					PathID: "/path/c",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 10},
					},
				},
				{
					PathID: "/path/a",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 1},
					},
				},
				{
					PathID: "/path/b",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 5},
					},
				},
			},
			expected: []string{"/path/a", "/path/b", "/path/c"},
		},
		{
			name: "Multiple component groups - uses minimum order",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 10},
						{Order: 2}, // Minimum is 2
						{Order: 15},
					},
				},
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 5},
						{Order: 3}, // Minimum is 3
						{Order: 8},
					},
				},
			},
			expected: []string{"/path/one", "/path/two"},
		},
		{
			name: "Equal minimum orders - maintains stable sort",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 5},
					},
				},
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 5},
					},
				},
			},
			expected: []string{"/path/one", "/path/two"},
		},
		{
			name: "Complex scenario with multiple groups and varying orders",
			input: []*entities.ScanResult{
				{
					PathID: "/path/high",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 100},
						{Order: 50},
					},
				},
				{
					PathID: "/path/best",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 1},
					},
				},
				{
					PathID: "/path/medium",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 20},
						{Order: 15},
						{Order: 25},
					},
				},
				{
					PathID: "/path/low",
					ComponentGroups: []*entities.ComponentGroup{
						{Order: 3},
						{Order: 5},
					},
				},
			},
			expected: []string{"/path/best", "/path/low", "/path/medium", "/path/high"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original
			input := make([]*entities.ScanResult, len(tt.input))
			copy(input, tt.input)

			// Sort the results
			sortByBestComponentOrder(input)

			// Verify the order
			if len(input) != len(tt.expected) {
				t.Errorf("Expected %d results, got %d", len(tt.expected), len(input))
				return
			}

			for i, expectedPath := range tt.expected {
				if input[i].PathID != expectedPath {
					t.Errorf("Position %d: expected PathID %s, got %s", i, expectedPath, input[i].PathID)
				}
			}
		})
	}
}

func TestHasHighScoreMatch(t *testing.T) {
	service := &ScanServiceImpl{}

	tests := []struct {
		name            string
		componentGroups []entities.ComponentGroup
		threshold       float32
		expected        bool
	}{
		{
			name:            "Empty component groups",
			componentGroups: []entities.ComponentGroup{},
			threshold:       0.5,
			expected:        false,
		},
		{
			name: "No versions meet threshold",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Score: 0.3},
						{Score: 0.4},
					},
				},
			},
			threshold: 0.5,
			expected:  false,
		},
		{
			name: "One version meets threshold",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Score: 0.3},
						{Score: 0.6},
					},
				},
			},
			threshold: 0.5,
			expected:  true,
		},
		{
			name: "Version equals threshold",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Score: 0.5},
					},
				},
			},
			threshold: 0.5,
			expected:  true,
		},
		{
			name: "Multiple groups, one has high score",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Score: 0.2},
					},
				},
				{
					Versions: []entities.Version{
						{Score: 0.9},
					},
				},
			},
			threshold: 0.8,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.hasHighScoreMatch(tt.componentGroups, tt.threshold)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestProcessComponentGroups(t *testing.T) {
	service := &ScanServiceImpl{}

	tests := []struct {
		name             string
		componentGroups  []entities.ComponentGroup
		path             string
		minAcceptedScore float32
		expectedCount    int // Number of results
		expectedVersions int // Number of versions in first group
	}{
		{
			name:             "Empty component groups",
			componentGroups:  []entities.ComponentGroup{},
			path:             "/test/path",
			minAcceptedScore: 0.5,
			expectedCount:    0,
			expectedVersions: 0,
		},
		{
			name: "All versions below threshold",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Version: "1.0.0", Score: 0.3},
						{Version: "2.0.0", Score: 0.4},
					},
				},
			},
			path:             "/test/path",
			minAcceptedScore: 0.5,
			expectedCount:    0,
			expectedVersions: 0,
		},
		{
			name: "Some versions above threshold",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Version: "1.0.0", Score: 0.3},
						{Version: "2.0.0", Score: 0.6},
						{Version: "3.0.0", Score: 0.8},
					},
				},
			},
			path:             "/test/path",
			minAcceptedScore: 0.5,
			expectedCount:    1,
			expectedVersions: 2,
		},
		{
			name: "All versions above threshold",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Version: "1.0.0", Score: 0.6},
						{Version: "2.0.0", Score: 0.7},
					},
				},
			},
			path:             "/test/path",
			minAcceptedScore: 0.5,
			expectedCount:    1,
			expectedVersions: 2,
		},
		{
			name: "Multiple groups, mixed results",
			componentGroups: []entities.ComponentGroup{
				{
					Versions: []entities.Version{
						{Version: "1.0.0", Score: 0.3},
					},
				},
				{
					Versions: []entities.Version{
						{Version: "2.0.0", Score: 0.8},
					},
				},
			},
			path:             "/test/path",
			minAcceptedScore: 0.5,
			expectedCount:    1,
			expectedVersions: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := service.processComponentGroups(tt.componentGroups, tt.path, tt.minAcceptedScore)

			if len(results) != tt.expectedCount {
				t.Errorf("Expected %d results, got %d", tt.expectedCount, len(results))
				return
			}

			if tt.expectedCount > 0 {
				if results[0].PathID != tt.path {
					t.Errorf("Expected PathID %s, got %s", tt.path, results[0].PathID)
				}

				totalVersions := 0
				for _, group := range results[0].ComponentGroups {
					totalVersions += len(group.Versions)
				}

				if totalVersions != tt.expectedVersions {
					t.Errorf("Expected %d versions, got %d", tt.expectedVersions, totalVersions)
				}
			}
		})
	}
}

func TestDeduplicateComponents(t *testing.T) {
	service := &ScanServiceImpl{}

	tests := []struct {
		name          string
		input         []*entities.ScanResult
		expectedCount int
		expectedPaths []string
	}{
		{
			name:          "Empty results",
			input:         []*entities.ScanResult{},
			expectedCount: 0,
			expectedPaths: []string{},
		},
		{
			name: "No duplicates",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{
							PURL:  "pkg:npm/component-a",
							Order: 1,
							Versions: []entities.Version{
								{Score: 0.8},
							},
						},
					},
				},
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{
							PURL:  "pkg:npm/component-b",
							Order: 2,
							Versions: []entities.Version{
								{Score: 0.7},
							},
						},
					},
				},
			},
			expectedCount: 2,
			expectedPaths: []string{"/path/one", "/path/two"},
		},
		{
			name: "Duplicate component - keep higher score",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{
							PURL:  "pkg:npm/component-a",
							Order: 2,
							Versions: []entities.Version{
								{Score: 0.6},
							},
						},
					},
				},
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{
							PURL:  "pkg:npm/component-a",
							Order: 1,
							Versions: []entities.Version{
								{Score: 0.9},
							},
						},
					},
				},
			},
			expectedCount: 1,
			expectedPaths: []string{"/path/two"},
		},
		{
			name: "Multiple duplicates across paths",
			input: []*entities.ScanResult{
				{
					PathID: "/path/one",
					ComponentGroups: []*entities.ComponentGroup{
						{
							PURL:  "pkg:npm/component-a",
							Order: 1,
							Versions: []entities.Version{
								{Score: 0.5},
							},
						},
						{
							PURL:  "pkg:npm/component-b",
							Order: 2,
							Versions: []entities.Version{
								{Score: 0.6},
							},
						},
					},
				},
				{
					PathID: "/path/two",
					ComponentGroups: []*entities.ComponentGroup{
						{
							PURL:  "pkg:npm/component-a",
							Order: 3,
							Versions: []entities.Version{
								{Score: 0.9},
							},
						},
					},
				},
			},
			expectedCount: 2,
			// After deduplication: /path/one keeps component-b (order 2), /path/two keeps component-a (order 3)
			// Sorted by order: /path/one (2) < /path/two (3)
			expectedPaths: []string{"/path/one", "/path/two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := service.deduplicateComponents(tt.input)

			if len(results) != tt.expectedCount {
				t.Errorf("Expected %d results, got %d", tt.expectedCount, len(results))
				return
			}

			// Verify paths match expected (already sorted by order)
			for i, expectedPath := range tt.expectedPaths {
				if results[i].PathID != expectedPath {
					t.Errorf("Position %d: expected PathID %s, got %s", i, expectedPath, results[i].PathID)
				}
			}
		})
	}
}
