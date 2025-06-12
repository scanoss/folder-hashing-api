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
	"flag"
	"fmt"
	"os"

	"github.com/golobby/config/v3"
	"github.com/golobby/config/v3/pkg/feeder"
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// Config is configuration for Server.
type Config struct {
	App struct {
		Name     string `env:"APP_NAME" envDefault:"SCANOSS HFH Server" json:"Name"`
		GRPCPort string `env:"APP_PORT" envDefault:"50061" json:"GRPCPort"`
		RESTPort string `env:"REST_PORT" envDefault:"40061" json:"RESTPort"`
		Debug    bool   `env:"APP_DEBUG" envDefault:"false" json:"Debug"`
		Trace    bool   `env:"APP_TRACE" envDefault:"false" json:"Trace"`
		Mode     string `env:"APP_MODE" envDefault:"dev" json:"Mode"`
	} `json:"App"`
	Logging struct {
		DynamicLogging bool   `env:"LOG_DYNAMIC" envDefault:"true" json:"DynamicLogging"`
		DynamicPort    string `env:"LOG_DYNAMIC_PORT" envDefault:"localhost:60061" json:"DynamicPort"`
		ConfigFile     string `env:"LOG_JSON_CONFIG" json:"ConfigFile"`
	} `json:"Logging"`
	Telemetry struct {
		Enabled      bool   `env:"OTEL_ENABLED" envDefault:"false" json:"Enabled"`
		OltpExporter string `env:"OTEL_EXPORTER_OLTP" envDefault:"0.0.0.0:4317" json:"OltpExporter"` // OTEL OLTP exporter (default 0.0.0.0:4317)
	} `json:"Telemetry"`
	TLS struct {
		CertFile string `env:"COMP_TLS_CERT" json:"CertFile"` // TLS Certificate
		KeyFile  string `env:"COMP_TLS_KEY" json:"KeyFile"`   // Private TLS Key
		CN       string `env:"COMP_TLS_CN" json:"CN"`         // Common Name (replaces the CN on the certificate)
	} `json:"TLS"`
	Filtering struct {
		AllowListFile  string `env:"COMP_ALLOW_LIST" json:"AllowListFile"`        // Allow list file for incoming connections
		DenyListFile   string `env:"COMP_DENY_LIST" json:"DenyListFile"`          // Deny list file for incoming connections
		BlockByDefault bool   `env:"COMP_BLOCK_BY_DEFAULT" json:"BlockByDefault"` // Block request by default if they are not in the allow list
		TrustProxy     bool   `env:"COMP_TRUST_PROXY" json:"TrustProxy"`          // Trust the interim proxy or not (causes the source IP to be validated instead of the proxy)
	} `json:"Filtering"`
	Hfh struct {
		QdrantHost string `env:"QDRANT_HOST" envDefault:"localhost" json:"QdrantHost"`
		QdrantPort int    `env:"QDRANT_PORT" envDefault:"6334" json:"QdrantPort"`
		// TODO: Discuss what else we need to add here as config options
	} `json:"Hfh"`
}

// LoadConfig loads configuration from command line flags, JSON file, .env file, and environment variables
// Priority order: Environment variables > JSON file > .env file > defaults
func LoadConfig() (*Config, error) {
	var jsonConfig, envConfig string
	debug := flag.Bool("debug", false, "Enable debug")
	version := flag.Bool("version", false, "Display current version")
	flag.StringVar(&jsonConfig, "json-config", "", "JSON configuration file path")
	flag.StringVar(&envConfig, "env-config", "", ".env configuration file path")
	flag.Parse()

	if *version {
		fmt.Printf("Version: %v", entities.AppVersion)
		os.Exit(1)
	}

	cfg := &Config{}

	c := config.New()

	// Add .env file (lowest priority)
	if envConfig != "" {
		c.AddFeeder(feeder.DotEnv{Path: envConfig})
	} else if _, err := os.Stat(".env"); err == nil {
		c.AddFeeder(feeder.DotEnv{Path: ".env"})
	}

	// Add JSON file if specified (medium priority)
	if jsonConfig != "" {
		c.AddFeeder(feeder.Json{Path: jsonConfig})
	}

	// Add environment variables (highest priority)
	c.AddFeeder(feeder.Env{})

	c.AddStruct(cfg)

	if err := c.Feed(); err != nil {
		return nil, err
	}

	// Debug flag override
	if *debug {
		cfg.App.Debug = true
		if err := os.Setenv("APP_DEBUG", "1"); err != nil {
			fmt.Printf("Warning: Failed to set env APP_DEBUG to 1: %v", err)
		}
	}

	return cfg, nil
}

func NewConfig(cfg *Config) *Config {
	return cfg
}
