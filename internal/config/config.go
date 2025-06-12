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

// Config is configuration for Server.
type Config struct {
	App struct {
		Name     string `env:"APP_NAME" envDefault:"SCANOSS HFH Server"`
		GRPCPort string `env:"APP_PORT" envDefault:"50061"`
		RESTPort string `env:"REST_PORT" envDefault:"40061"`
		Debug    bool   `env:"APP_DEBUG" envDefault:"false"`
		Trace    bool   `env:"APP_TRACE" envDefault:"false"`
		Mode     string `env:"APP_MODE" envDefault:"dev"`
	}
	Logging struct {
		DynamicLogging bool   `env:"LOG_DYNAMIC" envDefault:"true"`
		DynamicPort    string `env:"LOG_DYNAMIC_PORT" envDefault:"localhost:60061"`
		ConfigFile     string `env:"LOG_JSON_CONFIG"`
	}
	Telemetry struct {
		Enabled      bool   `env:"OTEL_ENABLED" envDefault:"false"`
		OltpExporter string `env:"OTEL_EXPORTER_OLTP" envDefault:"0.0.0.0:4317"` // OTEL OLTP exporter (default 0.0.0.0:4317)
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
	Hfh struct {
		QdrantHost string `env:"QDRANT_HOST" envDefault:"localhost"`
		QdrantPort int    `env:"QDRANT_PORT" envDefault:"6334"`
		// TODO: Discuss what else we need to add here as config options
	}
}

func NewConfig(cfg *Config) *Config {
	return cfg
}
