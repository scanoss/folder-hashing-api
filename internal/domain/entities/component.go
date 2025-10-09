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

// Package entities contains domain entities and data structures for the HFH service.
package entities

// ComponentGroup represents a grouped component with versions.
type ComponentGroup struct {
	PURL     string    `json:"purl"`
	Name     string    `json:"name"`
	Vendor   string    `json:"vendor"`
	Versions []Version `json:"versions"`
	Rank     int32     `json:"rank"`
	Order    int32     `json:"order"`
}

// Version represents a component version with score.
type Version struct {
	Version string  `json:"version"`
	Score   float32 `json:"score"`
}

// SearchResult represents a search result from Qdrant.
type SearchResult struct {
	Score     float32 `json:"score"`
	ID        uint64  `json:"id"`
	Vendor    string  `json:"vendor"`
	Component string  `json:"component"`
	Purl      string  `json:"purl"`
	Version   string  `json:"version"`
	Rank      int     `json:"rank"`
}
