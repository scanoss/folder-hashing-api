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
