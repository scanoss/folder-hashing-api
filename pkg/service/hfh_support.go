package service

import (
	"encoding/json"
	"errors"

	"github.com/scanoss/folder-hashing-api/pkg/dtos"
	pb "github.com/scanoss/papi/api/scanningv2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

// Structure for storing OTEL metrics.
type metricsCounters struct {
	hfhScanHistogram metric.Int64Histogram // milliseconds
}

var oltpMetrics = metricsCounters{}

// setupMetrics configures all the metrics recorders for the platform.
func setupMetrics() {
	meter := otel.Meter("github.com/scanoss/folder-hashing-api")
	oltpMetrics.hfhScanHistogram, _ = meter.Int64Histogram("hfh.scan.req_time", metric.WithDescription("The time taken to run a hfh scan request (ms)"))
}

func convertHFHscanInput(s *zap.SugaredLogger, request *pb.HFHRequest) (dtos.HFHscanInput, error) {
	data, err := json.Marshal(request)
	if err != nil {
		s.Errorf("Problem marshalling component request input: %v", err)
		return dtos.HFHscanInput{}, errors.New("problem marshalling component input")
	}
	dtoRequest, err := dtos.ParseHFHRequest(s, data)
	if err != nil {
		s.Errorf("Problem parsing component request input: %v", err)
		return dtos.HFHscanInput{}, errors.New("problem parsing component input")
	}
	return dtoRequest, nil
}

func convertHFHscanOutput(s *zap.SugaredLogger, output dtos.HFHResultOutput) (*pb.HFHResponse, error) {
	data, err := json.Marshal(output)
	if err != nil {
		s.Errorf("Problem marshalling component request output: %v", err)
		return &pb.HFHResponse{}, errors.New("problem marshalling component output")
	}
	var compResp pb.HFHResponse
	err = json.Unmarshal(data, &compResp)
	if err != nil {
		s.Errorf("Problem unmarshalling component request output: %v", err)
		return &pb.HFHResponse{}, errors.New("problem unmarshalling component output")
	}
	return &compResp, nil
}
