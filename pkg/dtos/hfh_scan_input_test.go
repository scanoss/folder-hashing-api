package dtos

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	zlog "github.com/scanoss/zap-logging-helper/pkg/logger"
)

func TestParseHFHRequest(t *testing.T) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a sugared logger", err)
	}
	defer zlog.SyncZap()
	ctx := ctxzap.ToContext(context.Background(), zlog.L)
	s := ctxzap.Extract(ctx).Sugar()

	goodTest := []struct {
		input string
		want  HFHscanInput
	}{
		{
			input: `{
				"best_match": true,
				"threshold": 0.8,
				"root": {
					"path_id": "root_1",
					"sim_hash_names": "hash_root_1",
					"sim_hash_content": "content_root_1",
					"children": [
						{
							"path_id": "level1_1",
							"sim_hash_names": "hash_l1_1",
							"sim_hash_content": "content_l1_1",
							"children": [
								{
									"path_id": "level2_1",
									"sim_hash_names": "hash_l2_1",
									"sim_hash_content": "content_l2_1",
									"children": [
										{
											"path_id": "level3_1",
											"sim_hash_names": "hash_l3_1",
											"sim_hash_content": "content_l3_1"
										},
										{
											"path_id": "level3_2",
											"sim_hash_names": "hash_l3_2",
											"sim_hash_content": "content_l3_2"
										}
									]
								},
								{
									"path_id": "level2_2",
									"sim_hash_names": "hash_l2_2",
									"sim_hash_content": "content_l2_2"
								}
							]
						},
						{
							"path_id": "level1_2",
							"sim_hash_names": "hash_l1_2",
							"sim_hash_content": "content_l1_2",
							"children": [
								{
									"path_id": "level2_3",
									"sim_hash_names": "hash_l2_3",
									"sim_hash_content": "content_l2_3"
								}
							]
						}
					]
				}
			}`,
			want: HFHscanInput{
				BestMatch: true,
				Threshold: 0.8,
				Root: &HFHScanInputChildren{
					PathId:         "root_1",
					SimHashNames:   "hash_root_1",
					SimHashContent: "content_root_1",
					Children: []*HFHScanInputChildren{
						{
							PathId:         "level1_1",
							SimHashNames:   "hash_l1_1",
							SimHashContent: "content_l1_1",
							Children: []*HFHScanInputChildren{
								{
									PathId:         "level2_1",
									SimHashNames:   "hash_l2_1",
									SimHashContent: "content_l2_1",
									Children: []*HFHScanInputChildren{
										{
											PathId:         "level3_1",
											SimHashNames:   "hash_l3_1",
											SimHashContent: "content_l3_1",
										},
										{
											PathId:         "level3_2",
											SimHashNames:   "hash_l3_2",
											SimHashContent: "content_l3_2",
										},
									},
								},
								{
									PathId:         "level2_2",
									SimHashNames:   "hash_l2_2",
									SimHashContent: "content_l2_2",
								},
							},
						},
						{
							PathId:         "level1_2",
							SimHashNames:   "hash_l1_2",
							SimHashContent: "content_l1_2",
							Children: []*HFHScanInputChildren{
								{
									PathId:         "level2_3",
									SimHashNames:   "hash_l2_3",
									SimHashContent: "content_l2_3",
								},
							},
						},
					},
				},
			},
		},
		{
			input: `{
				"best_match": true,
				"threshold": 0.7,
				"root": {
					"path_id": "root_2",
					"children": [
						{
							"path_id": "child1",
							"children": [
								{
									"path_id": "child1.1"
								},
								{
									"path_id": "child1.2"
								}
							]
						}
					]
				}
			}`,
			want: HFHscanInput{
				BestMatch: true,
				Threshold: 0.7,
				Root: &HFHScanInputChildren{
					PathId: "root_2",
					Children: []*HFHScanInputChildren{
						{
							PathId: "child1",
							Children: []*HFHScanInputChildren{
								{
									PathId: "child1.1",
								},
								{
									PathId: "child1.2",
								},
							},
						},
					},
				},
			},
		},
		{
			input: `{"best_match": true, "threshold": 0.7}`,
			want: HFHscanInput{
				BestMatch: true,
				Threshold: 0.7,
			},
		},
		{
			input: `{}`,
			want:  HFHscanInput{},
		},
	}

	badTest := []struct {
		input       string
		description string
	}{
		{
			description: "Broken JSON, missing comma",
			input:       `{"best_match": true "threshold": 0.8}`,
		},
		{
			description: "Invalid threshold type",
			input:       `{"best_match": true, "threshold": "invalid"}`,
		},
		{
			description: "Invalid children structure",
			input:       `{"best_match": true, "threshold": 0.8, "root": {"children": "invalid"}}`,
		},
		{
			description: "Invalid nested children",
			input:       `{"root": {"children": [{"children": "not-an-array"}]}}`,
		},
	}

	for _, test := range goodTest {
		res, err := ParseHFHRequest(s, []byte(test.input))
		if (!cmp.Equal(test.want, res)) || (err != nil) {
			t.Errorf("Error testing dto: %v\n. Wanted %v, Input: %v \n", err, test.want, test.input)
		}
	}

	for _, test := range badTest {
		if _, err := ParseHFHRequest(s, []byte(test.input)); err == nil {
			t.Errorf("Expected an error for input: %v - %v", test.description, test.input)
		}
	}

	_, err = ParseHFHRequest(s, []byte(""))
	if err == nil {
		t.Errorf("Expected an error for empty input")
	}

	_, err = ParseHFHRequest(s, nil)
	if err == nil {
		t.Errorf("Expected an error for nil input")
	}
}

func TestExportParseRequest(t *testing.T) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a sugared logger", err)
	}
	defer zlog.SyncZap()
	ctx := ctxzap.ToContext(context.Background(), zlog.L)
	s := ctxzap.Extract(ctx).Sugar()

	testCases := []struct {
		name  string
		input HFHscanInput
	}{
		{
			name: "Complex nested structure",
			input: HFHscanInput{
				BestMatch: true,
				Threshold: 0.8,
				Root: &HFHScanInputChildren{
					PathId:         "root",
					SimHashNames:   "root_hash",
					SimHashContent: "root_content",
					Children: []*HFHScanInputChildren{
						{
							PathId:         "level1_1",
							SimHashNames:   "l1_1_hash",
							SimHashContent: "l1_1_content",
							Children: []*HFHScanInputChildren{
								{
									PathId:         "level2_1",
									SimHashNames:   "l2_1_hash",
									SimHashContent: "l2_1_content",
									Children: []*HFHScanInputChildren{
										{
											PathId:         "level3_1",
											SimHashNames:   "l3_1_hash",
											SimHashContent: "l3_1_content",
										},
									},
								},
							},
						},
						{
							PathId:         "level1_2",
							SimHashNames:   "l1_2_hash",
							SimHashContent: "l1_2_content",
						},
					},
				},
			},
		},
		{
			name:  "Empty structure",
			input: HFHscanInput{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bytes, err := ExportParseRequest(s, tc.input)
			if err != nil {
				t.Errorf("Failed to export request: %v\n", err)
			}
			fmt.Printf("Converting %s to bytes: %v\n", tc.name, string(bytes))

			// Verify the exported data can be parsed back
			parsed, err := ParseHFHRequest(s, bytes)
			if err != nil {
				t.Errorf("Failed to parse exported data: %v\n", err)
			}
			if !cmp.Equal(tc.input, parsed) {
				t.Errorf("Export/Parse cycle failed. Original: %v, Got: %v", tc.input, parsed)
			}
		})
	}
}
