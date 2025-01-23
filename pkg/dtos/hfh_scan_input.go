package dtos

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// HFHRequest
type HFHscanInput struct {
	BestMatch bool                  `json:"best_match"`
	Threshold float32               `json:"threshold"`
	Root      *HFHScanInputChildren `json:"root,omitempty"`
}

// HFHRequestChildren represents the children nodes structure in the folder tree
type HFHScanInputChildren struct {
	PathId         string                  `json:"path_id,omitempty"`
	SimHashNames   string                  `json:"sim_hash_names,omitempty"`
	SimHashContent string                  `json:"sim_hash_content,omitempty"`
	Children       []*HFHScanInputChildren `json:"children,omitempty"`
}

func ParseHFHRequest(s *zap.SugaredLogger, input []byte) (HFHscanInput, error) {
	if len(input) == 0 {
		return HFHscanInput{}, errors.New("no data supplied to parse")
	}
	var data HFHscanInput
	err := json.Unmarshal(input, &data)
	if err != nil {
		s.Errorf("Parse failure: %v", err)
		return HFHscanInput{}, fmt.Errorf("failed to parse data: %v", err)
	}
	return data, nil
}

func ExportParseRequest(s *zap.SugaredLogger, input HFHscanInput) ([]byte, error) {
	data, err := json.Marshal(input)
	if err != nil {
		s.Errorf("Parse failure: %v", err)
		return nil, errors.New("failed to produce JSON ")
	}
	return data, nil
}
