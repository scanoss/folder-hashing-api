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

package config

import (
	"github.com/golobby/config/v3"
	"github.com/golobby/config/v3/pkg/feeder"
)

const (
	defaultGrpcPort = "50061"
	defaultRestPort = "40061"
)

// ServerConfig is configuration for Server.
type ServerConfig struct {
	App struct {
		Name     string `env:"APP_NAME"`
		GRPCPort string `env:"APP_PORT"`
		RESTPort string `env:"REST_PORT"`
		Debug    bool   `env:"APP_DEBUG"` // true/false
		Trace    bool   `env:"APP_TRACE"` // true/false
		Mode     string `env:"APP_MODE"`  // dev or prod
	}
	Logging struct {
		DynamicLogging bool   `env:"LOG_DYNAMIC"`      // true/false
		DynamicPort    string `env:"LOG_DYNAMIC_PORT"` // host:port
		ConfigFile     string `env:"LOG_JSON_CONFIG"`
	}
	Telemetry struct {
		Enabled      bool   `env:"OTEL_ENABLED"`       // true/false
		OltpExporter string `env:"OTEL_EXPORTER_OLTP"` // OTEL OLTP exporter (default 0.0.0.0:4317)
	}
	TLS struct {
		CertFile string `env:"COMP_TLS_CERT"` // TLS Certificate
		KeyFile  string `env:"COMP_TLS_KEY"`  // Private TLS Key
		CN       string `env:"COMP_TLS_CN"`   // Common Name (replaces the CN on the certificate)
	}
	Filtering struct {
		AllowListFile  string `env:"COMP_ALLOW_LIST"`       // Allow list file for incoming connections
		DenyListFile   string `env:"COMP_DENY_LIST"`        // Deny list file for incoming connections
		BlockByDefault bool   `env:"COMP_BLOCK_BY_DEFAULT"` // Block request by default if they are not in the allow list
		TrustProxy     bool   `env:"COMP_TRUST_PROXY"`      // Trust the interim proxy or not (causes the source IP to be validated instead of the proxy)
	}
	Ldb struct {
		Path   string `env:"LDB_PATH"` // LDB working path
		KbName string `env:"LDB_KB"`   // LDB KB name (must be inside the working path)
	}
	Hfh struct {
		Dmax       int     `env:"HFH_DMAX"`       // HFH maximum distanse to consider a candidate
		Threshold1 float32 `env:"HFH_TH1"`        // HFH first stafge analysis threshold
		Threshold2 float32 `env:"HFH_TH2"`        // HFH second stafge analysis threshold
		Threshold3 float32 `env:"HFH_TH3"`        // HFH third stafge analysis threshold
		SectorTol  int     `env:"HFH_SECTOR_TOL"` // HFH ldb sector tolerance
	}
}

// NewServerConfig loads all config options and return a struct for use.
func NewServerConfig(feeders []config.Feeder) (*ServerConfig, error) {
	cfg := ServerConfig{}
	setServerConfigDefaults(&cfg)
	c := config.New()
	for _, f := range feeders {
		c.AddFeeder(f)
	}
	c.AddFeeder(feeder.Env{})
	c.AddStruct(&cfg)
	err := c.Feed()
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// setServerConfigDefaults attempts to set reasonable defaults for the server config.
func setServerConfigDefaults(cfg *ServerConfig) {
	cfg.App.Name = "SCANOSS HFH Server"
	cfg.App.GRPCPort = defaultGrpcPort
	cfg.App.RESTPort = defaultRestPort
	cfg.App.Mode = "dev"
	cfg.App.Debug = false
	cfg.Logging.DynamicLogging = true
	cfg.Logging.DynamicPort = "localhost:60061"
	cfg.Telemetry.Enabled = false
	cfg.Telemetry.OltpExporter = "0.0.0.0:4317" // Default OTEL OLTP gRPC Exporter endpoint
	cfg.Ldb.Path = "/var/lib/ldb"
	cfg.Ldb.KbName = "hfh_kb"
	cfg.Hfh.Dmax = 24
	cfg.Hfh.SectorTol = 8
	cfg.Hfh.Threshold1 = 80
	cfg.Hfh.Threshold2 = 70
	cfg.Hfh.Threshold3 = 51
}
