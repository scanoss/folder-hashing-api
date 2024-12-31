package service

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
func setupMetrics() {
	meter := otel.Meter("scanoss.com/hfh-api")
	oltpMetrics.hfhScanHistogram, _ = meter.Int64Histogram("hfh.scan.req_time", metric.WithDescription("The time taken to run a hfh scan request (ms)"))
}
