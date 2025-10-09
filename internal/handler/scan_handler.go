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

// Package handler implements gRPC and REST request handlers for the HFH service.
package handler

import (
	"context"
	"errors"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/scanoss/papi/api/commonv2"
	"github.com/scanoss/papi/api/scanningv2"

	"github.com/scanoss/folder-hashing-api/internal/mapper"
	"github.com/scanoss/folder-hashing-api/internal/service"
	"github.com/scanoss/folder-hashing-api/internal/validation"
)

// ScanHandler implements the gRPC scanning service.
type ScanHandler struct {
	scanningv2.UnimplementedScanningServer
	scanService service.ScanService
	mapper      mapper.ScanMapper
}

// NewScanHandler creates a new scan handler.
func NewScanHandler(scanService service.ScanService, mapper mapper.ScanMapper) *ScanHandler {
	return &ScanHandler{
		scanService: scanService,
		mapper:      mapper,
	}
}

// FolderHashScan performs folder hash scanning.
func (h *ScanHandler) FolderHashScan(ctx context.Context, req *scanningv2.HFHRequest) (*scanningv2.HFHResponse, error) {
	requestStartTime := time.Now()
	s := ctxzap.Extract(ctx).Sugar()
	s.Info("Processing folder hashing scan request...")

	// Validate request
	if req.Root == nil {
		statusResp := commonv2.StatusResponse{
			Status:  commonv2.StatusCode_FAILED,
			Message: "There is no data to scan",
		}
		return &scanningv2.HFHResponse{Status: &statusResp}, errors.New("there is no data to scan")
	}

	s.Infof("Processing folder details: %+v", req)

	// Convert protobuf request to domain model
	domainRequest := h.mapper.ProtoToDomain(req)
	if domainRequest == nil {
		statusResp := commonv2.StatusResponse{
			Status:  commonv2.StatusCode_FAILED,
			Message: "Failed to process request",
		}
		return &scanningv2.HFHResponse{Status: &statusResp}, errors.New("failed to process request")
	}

	// Validate domain request
	if err := validation.ValidateStruct(domainRequest); err != nil {
		s.Errorf("validation error: %v", err)
		statusResp := commonv2.StatusResponse{
			Status:  commonv2.StatusCode_FAILED,
			Message: "Invalid request: " + err.Error(),
		}
		return &scanningv2.HFHResponse{Status: &statusResp}, err
	}

	s.Info("Scan starts")

	domainResponse, err := h.scanService.ScanFolder(ctx, domainRequest)
	if err != nil {
		s.Errorf("error during hfh scanning: %v", err)
		statusResp := commonv2.StatusResponse{
			Status:  commonv2.StatusCode_FAILED,
			Message: "Scan failed",
		}
		return &scanningv2.HFHResponse{Status: &statusResp}, err
	}

	// Convert domain response back to protobuf
	response := h.mapper.DomainToProto(domainResponse)
	if response == nil {
		return &scanningv2.HFHResponse{
			Status: &commonv2.StatusResponse{
				Status:  commonv2.StatusCode_FAILED,
				Message: "Failed to process response",
			},
		}, errors.New("failed to process response")
	}

	// Set success status
	response.Status = &commonv2.StatusResponse{
		Status:  commonv2.StatusCode_SUCCESS,
		Message: "Success",
	}

	elapsedTime := time.Since(requestStartTime)
	s.Infof("HFH scan completed in %v", elapsedTime)
	s.Infof("HFH response: %+v", response)

	return response, nil
}

// Echo sends back the same message received.
func (h *ScanHandler) Echo(ctx context.Context, req *commonv2.EchoRequest) (*commonv2.EchoResponse, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Infof("Received echo (%v): %v", ctx, req.GetMessage())
	return &commonv2.EchoResponse{
		Message: req.GetMessage(),
	}, nil
}
