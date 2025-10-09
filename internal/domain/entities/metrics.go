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

package entities

import (
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Structure for storing OTEL metrics.
type metricsCounters struct {
	hfhScanHistogram metric.Int64Histogram // milliseconds
}

var oltpMetrics = metricsCounters{}

// SetupMetrics configures all the metrics recorders for the platform.
func SetupMetrics() {
	meter := otel.Meter("github.com/scanoss/folder-hashing-api")
	var err error
	oltpMetrics.hfhScanHistogram, err = meter.Int64Histogram("hfh.scan.req_time", metric.WithDescription("The time taken to run a hfh scan request (ms)"))
	if err != nil {
		// Log error but don't fail - metrics are non-critical
		log.Printf("failed to create hfh.scan.req_time histogram: %v", err)
	}
}
