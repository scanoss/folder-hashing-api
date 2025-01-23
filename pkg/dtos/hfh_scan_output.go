package dtos

import (
	"encoding/json"
	"errors"

	"go.uber.org/zap"
)

// HFHResultOutput represents a collection of HFH results
type HFHResultOutput struct {
	Results []*HFHResult `json:"results,omitempty"`
}

// HFHResult represents a result item that links a path with a list of components
type HFHResult struct {
	PathId     string          `json:"path_id,omitempty"`
	Components []*HFHComponent `json:"components,omitempty"`
}

// HFHComponent represents a matched component with its details
type HFHComponent struct {
	Purl       string   `json:"purl,omitempty"`
	Versions   []string `json:"versions,omitempty"`
	Confidence float32  `json:"confidence,omitempty"`
}

func ExportHFHresult(s *zap.SugaredLogger, output HFHResultOutput) ([]byte, error) {
	data, err := json.Marshal(output)
	if err != nil {
		s.Errorf("Parse failure: %v", err)
		return nil, errors.New("failed to produce JSON")
	}
	return data, nil
}
