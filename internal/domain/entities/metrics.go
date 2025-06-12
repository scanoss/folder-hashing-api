package entities

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Structure for storing OTEL metrics.
type metricsCounters struct {
	hfhScanHistogram metric.Int64Histogram // milliseconds
}

var oltpMetrics = metricsCounters{}

// setupMetrics configures all the metrics recorders for the platform.
func SetupMetrics() {
	meter := otel.Meter("github.com/scanoss/folder-hashing-api")
	oltpMetrics.hfhScanHistogram, _ = meter.Int64Histogram("hfh.scan.req_time", metric.WithDescription("The time taken to run a hfh scan request (ms)"))
}
