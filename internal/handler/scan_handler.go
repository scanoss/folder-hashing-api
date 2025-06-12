package handler

import (
	"context"
	"errors"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/scanoss/folder-hashing-api/internal/mapper"
	"github.com/scanoss/folder-hashing-api/internal/service"
	"github.com/scanoss/papi/api/commonv2"
	"github.com/scanoss/papi/api/scanningv2"
)

// ScanHandler implements the gRPC scanning service
type ScanHandler struct {
	scanningv2.UnimplementedScanningServer
	scanService service.ScanService
	mapper      mapper.ScanMapper
}

// NewScanHandler creates a new scan handler
func NewScanHandler(scanService service.ScanService, mapper mapper.ScanMapper) *ScanHandler {
	return &ScanHandler{
		scanService: scanService,
		mapper:      mapper,
	}
}

// FolderHashScan performs folder hash scanning
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

	s.Info("Scan starts")

	// Perform the scan using the service layer
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
		statusResp := commonv2.StatusResponse{
			Status:  commonv2.StatusCode_FAILED,
			Message: "Failed to process response",
		}
		return &scanningv2.HFHResponse{Status: &statusResp}, errors.New("failed to process response")
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

// Echo sends back the same message received
func (h *ScanHandler) Echo(ctx context.Context, req *commonv2.EchoRequest) (*commonv2.EchoResponse, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Infof("Received echo (%v): %v", ctx, req.GetMessage())
	return &commonv2.EchoResponse{
		Message: req.GetMessage(),
	}, nil
}
