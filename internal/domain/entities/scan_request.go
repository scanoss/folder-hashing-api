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

package entities

// ScanRequest represents the domain model for folder hash scanning request.
type ScanRequest struct {
	// Get results with rank above this threshold (e.g i only want to see results from rank 3 and above)
	RankThreshold int `validate:"omitempty,min=0"`
	// Recursive threshold (e.g i only want to see results with score above this threshold)
	RecursiveThreshold float32 `validate:"omitempty,min=0"`
	// Minimum accepted score - only matches with score bigger than this value will be reported (default: 0.15)
	MinAcceptedScore float32 `validate:"omitempty,min=0"`
	// Filter results by category (e.g i only want to see results from github projects, npm, etc)
	Category string
	// Maximum number of results to query
	QueryLimit int `validate:"omitempty,min=1"`
	// Folder root node to be scanned
	Root *FolderNode `validate:"required"`
}

// FolderNode represents a folder node in the hierarchical structure.
type FolderNode struct {
	// Folder path (can be actual or obfuscated)
	PathID string `validate:"required"`
	// Proximity hash calculated from this nodes filenames (and their children)
	SimHashNames string `validate:"required"`
	// Proximity hash calculated from this nodes file contents (and their children)
	SimHashContent string `validate:"required"`
	// Proximity hash calculated from this nodes directory names (and their children)
	SimHashDirNames string `validate:"required"`
	// Language extensions count (dictionary) - language name -> count
	LangExtensions LanguageExtensions
	// Sub-folders inside this child
	Children []*FolderNode
}

// LanguageExtensions represents file extension counts by language.
type LanguageExtensions map[string]int32
