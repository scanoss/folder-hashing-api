// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2018-2022 SCANOSS.COM
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

// Package service implements the gRPC service endpoints
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	common "github.com/scanoss/papi/api/commonv2"
	pb "github.com/scanoss/papi/api/scanningv2"
	myconfig "scanoss.com/hfh-api/pkg/config"
	u "scanoss.com/hfh-api/pkg/usecase"
)

type hfhServer struct {
	pb.ScanningServer
	config        *myconfig.ServerConfig
	scannerConfig *u.HFHscanConfig
}

func NewFolderHashingServer(config *myconfig.ServerConfig) (*hfhServer, error) {
	setupMetrics()
	scannerConfig := u.HFHScanInit(config)
	if scannerConfig == nil {
		return nil, fmt.Errorf("error creating scanning instance")
	}
	return &hfhServer{config: config, scannerConfig: scannerConfig}, nil
}

// Echo sends back the same message received.
func (d hfhServer) Echo(ctx context.Context, request *common.EchoRequest) (*common.EchoResponse, error) {
	s := ctxzap.Extract(ctx).Sugar()
	s.Infof("Received (%v): %v", ctx, request.GetMessage())
	return &common.EchoResponse{Message: request.GetMessage()}, nil
}

// FolderHashScan and searches for the best component matches for the given folder.
func (d hfhServer) FolderHashScan(ctx context.Context, request *pb.HFHRequest) (*pb.HFHResponse, error) {
	requestStartTime := time.Now() // Capture the scan start time
	s := ctxzap.Extract(ctx).Sugar()
	s.Info("Processing folder hashing scan request...")
	if request.Root == nil {
		statusResp := common.StatusResponse{Status: common.StatusCode_FAILED, Message: "There is no data to scan"}
		return &pb.HFHResponse{Status: &statusResp}, errors.New("there is no data to scan")
	}
	// TODO add use case to run folder scanning
	s.Infof("Processing folder details: %+v", request)
	dtoRequest, err := convertHFHscanInput(s, request)
	if err != nil {
		return nil, fmt.Errorf("error processing request")
	}
	scanner := u.HFHScanNew(s, d.scannerConfig, &dtoRequest)
	s.Infof("Scan starts")
	dtoResults, err := scanner.Scan(dtoRequest.Root)
	if err != nil {
		s.Errorf("error during hfh scanning: %v", err)
		statusResp := common.StatusResponse{Status: common.StatusCode_FAILED, Message: "Failure"}
		return &pb.HFHResponse{Status: &statusResp}, err
	}
	results, err := convertHFHscanOutput(s, dtoResults)
	if err != nil {
		return nil, fmt.Errorf("error processing response")
	}
	s.Infof("HFH response: %+v", results)
	telemetryHfhScanRequestTime(ctx, d.config, requestStartTime)
	// Set the status and respond with the data
	results.Status = &common.StatusResponse{Status: common.StatusCode_SUCCESS, Message: "Success"}

	return results, nil
}

// telemetryHfhScanRequestTime records the versions request time to telemetry.
func telemetryHfhScanRequestTime(ctx context.Context, config *myconfig.ServerConfig, requestStartTime time.Time) {
	if config.Telemetry.Enabled {
		elapsedTime := time.Since(requestStartTime).Milliseconds() // Time taken to run the hfh scan request
		oltpMetrics.hfhScanHistogram.Record(ctx, elapsedTime)      // Record dep request time
	}
}
