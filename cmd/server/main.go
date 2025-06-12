// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2018-2024 SCANOSS.COM
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

// Package main loads the gRPC High Precision Folder Hashing Server Service
package main

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/scanoss/folder-hashing-api/internal/config"
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/folder-hashing-api/internal/handler"
	"github.com/scanoss/folder-hashing-api/internal/mapper"
	"github.com/scanoss/folder-hashing-api/internal/protocol/grpc"
	"github.com/scanoss/folder-hashing-api/internal/protocol/rest"
	"github.com/scanoss/folder-hashing-api/internal/repository"
	"github.com/scanoss/folder-hashing-api/internal/service"
	"github.com/scanoss/go-grpc-helper/pkg/files"
	gs "github.com/scanoss/go-grpc-helper/pkg/grpc/server"
	zlog "github.com/scanoss/zap-logging-helper/pkg/logger"
)

// main starts the gRPC HFH Service.
func main() {
	cfg := config.Config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("ERROR: Environment variables parsing error: %v\n", err)
	}

	if err := zlog.SetupAppLogger(cfg.App.Mode, cfg.Logging.ConfigFile, cfg.App.Debug); err != nil {
		log.Fatalf("ERROR: Logger setup error: %v\n", err)
	}
	defer zlog.SyncZap()

	// Check if TLS/SSL should be enabled
	startTLS, err := files.CheckTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	if err != nil {
		log.Fatalf("ERROR: TLS check error: %v\n", err)
	}

	// Check if IP filtering should be enabled
	allowedIPs, deniedIPs, err := files.LoadFiltering(cfg.Filtering.AllowListFile, cfg.Filtering.DenyListFile)
	if err != nil {
		log.Fatalf("ERROR: IP filtering error: %v\n", err)
	}

	zlog.S.Infof("Starting SCANOSS HFH Service: %v", strings.TrimSpace(entities.AppVersion))

	// Setup dynamic logging (if necessary)
	zlog.SetupAppDynamicLogging(cfg.Logging.DynamicPort, cfg.Logging.DynamicLogging)

	ctx := context.Background()

	// Initialize metrics
	entities.SetupMetrics()

	// Create repository
	scanRepo, err := repository.NewScanRepositoryQdrantImpl(repository.QdrantConfig{
		Host: cfg.Hfh.QdrantHost,
		Port: cfg.Hfh.QdrantPort,
	})
	if err != nil {
		log.Fatalf("ERROR: failed to create scan repository: %v\n", err)
	}
	zlog.S.Infof("Connected to Qdrant at %s:%d", cfg.Hfh.QdrantHost, cfg.Hfh.QdrantPort)
	scanService := service.NewScanService(scanRepo)
	scanMapper := mapper.NewScanMapper()
	scanHandler := handler.NewScanHandler(scanService, scanMapper)

	// Start the REST grpc-gateway if requested
	var srv *http.Server
	if len(cfg.App.RESTPort) > 0 {
		if srv, err = rest.RunServer(&cfg, ctx, cfg.App.GRPCPort, cfg.App.RESTPort, allowedIPs, deniedIPs, startTLS); err != nil {
			log.Fatalf("ERROR: REST server setup error: %v\n", err)
		}
	}

	// Start the gRPC service
	server, err := grpc.RunServer(&cfg, scanHandler, cfg.App.GRPCPort, allowedIPs, deniedIPs, startTLS, entities.AppVersion)
	if err != nil {
		log.Fatalf("ERROR: gRPC server setup error: %v\n", err)
	}

	// graceful shutdown
	if err := gs.WaitServerComplete(srv, server); err != nil {
		log.Fatalf("ERROR: gRPC server shutdown error: %v\n", err)
	}
}
