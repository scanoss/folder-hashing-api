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

// Package mapper provides conversion between protobuf and domain models.
package mapper

import (
	"github.com/scanoss/papi/api/scanningv2"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// ScanMapper converts between protobuf and domain scan models.
type ScanMapper interface {
	ProtoToDomain(req *scanningv2.HFHRequest) *entities.ScanRequest
	DomainToProto(resp *entities.ScanResponse) *scanningv2.HFHResponse
	ChildrenToDomain(children *scanningv2.HFHRequest_Children) *entities.FolderNode
}
