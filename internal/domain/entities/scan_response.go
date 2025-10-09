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

// ScanResponse represents the domain model for folder hash scanning response.
type ScanResponse struct {
	// List of folders containing unique components
	Results []*ScanResult
}

// ScanResult represents a scan result for a specific path.
type ScanResult struct {
	// Folder path (can be actual or obfuscated)
	PathID string
	// List of matching component groups
	ComponentGroups []*ComponentGroup
}

// StatusResponse represents status information.
type StatusResponse struct {
	Code    int32
	Message string
}
