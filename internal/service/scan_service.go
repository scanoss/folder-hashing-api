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

// Package service implements business logic for the HFH service.
package service

import (
	"context"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// ScanService defines the interface for folder hashing scan operations.
type ScanService interface {
	// ScanFolder performs a folder hashing scan
	ScanFolder(ctx context.Context, req *entities.ScanRequest) (*entities.ScanResponse, error)
}
