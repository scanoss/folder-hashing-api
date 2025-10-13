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

// Package grpc handles all the gRPC communication for the Folder Hashing Service
// It takes care of starting and stopping the listener, etc.
package grpc

import (
	"github.com/scanoss/go-grpc-helper/pkg/grpc/otel"
	gs "github.com/scanoss/go-grpc-helper/pkg/grpc/server"
	pb "github.com/scanoss/papi/api/scanningv2"
	"google.golang.org/grpc"

	"github.com/scanoss/folder-hashing-api/internal/config"
)

// RunServer runs gRPC service to publish.
func RunServer(cfg *config.Config, handler pb.ScanningServer, port string, allowedIPs, deniedIPs []string, startTLS bool, version string) (*grpc.Server, error) {
	// Start up Open Telemetry is requested
	oltpShutdown := func() {}
	if cfg.Telemetry.Enabled {
		var err error
		oltpShutdown, err = otel.InitTelemetryProviders(cfg.App.Name, "scanoss-hfh", version,
			cfg.Telemetry.OltpExporter, otel.GetTraceSampler(cfg.App.Mode), false)
		if err != nil {
			return nil, err
		}
	}

	// Configure the port, interceptors, TLS and register the service
	listen, server, err := gs.SetupGrpcServer(
		port,
		cfg.TLS.CertFile,
		cfg.TLS.KeyFile,
		allowedIPs,
		deniedIPs,
		startTLS,
		cfg.Filtering.BlockByDefault,
		cfg.Filtering.TrustProxy,
		cfg.Telemetry.Enabled,
	)
	if err != nil {
		oltpShutdown()
		return nil, err
	}

	// Register the service API and start the server in the background
	pb.RegisterScanningServer(server, handler)

	go func() {
		gs.StartGrpcServer(listen, server, startTLS)
		oltpShutdown()
	}()

	return server, nil
}
