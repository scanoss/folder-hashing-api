package dtos

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	zlog "github.com/scanoss/zap-logging-helper/pkg/logger"
)

func TestExportHFHresult(t *testing.T) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a sugared logger", err)
	}
	defer zlog.SyncZap()
	ctx := ctxzap.ToContext(context.Background(), zlog.L)
	s := ctxzap.Extract(ctx).Sugar()

	testCases := []struct {
		name   string
		input  HFHResultOutput
		expect string
	}{
		{
			name: "Complex structure with multiple paths and components",
			input: HFHResultOutput{
				Results: []*HFHResult{
					{
						PathId: "root/src/main",
						Components: []*HFHComponent{
							{
								Purl:     "pkg:npm/react@18.2.0",
								Versions: []string{"18.2.0", "18.1.0", "18.0.0"},
								Rank:     5,
							},
							{
								Purl:     "pkg:npm/lodash@4.17.21",
								Versions: []string{"4.17.21", "4.17.20"},
								Rank:     6,
							},
						},
					},
					{
						PathId: "root/src/components",
						Components: []*HFHComponent{
							{
								Purl:     "pkg:npm/@material-ui/core@4.12.4",
								Versions: []string{"4.12.4", "4.12.3", "4.12.2"},
								Rank:     4,
							},
						},
					},
					{
						PathId: "root/src/utils",
						Components: []*HFHComponent{
							{
								Purl:     "pkg:npm/axios@1.3.4",
								Versions: []string{"1.3.4", "1.3.3"},
								Rank:     5,
							},
							{
								Purl:     "pkg:npm/moment@2.29.4",
								Versions: []string{"2.29.4", "2.29.3"},
								Rank:     6,
							},
							{
								Purl:     "pkg:npm/uuid@9.0.0",
								Versions: []string{"9.0.0", "8.3.2"},
								Rank:     6,
							},
						},
					},
				},
			},
			expect: `{"results":[{"path_id":"root/src/main","components":[{"purl":"pkg:npm/react@18.2.0","versions":["18.2.0","18.1.0","18.0.0"],"rank":5},{"purl":"pkg:npm/lodash@4.17.21","versions":["4.17.21","4.17.20"],"rank":6}]},{"path_id":"root/src/components","components":[{"purl":"pkg:npm/@material-ui/core@4.12.4","versions":["4.12.4","4.12.3","4.12.2"],"rank":4}]},{"path_id":"root/src/utils","components":[{"purl":"pkg:npm/axios@1.3.4","versions":["1.3.4","1.3.3"],"rank":5},{"purl":"pkg:npm/moment@2.29.4","versions":["2.29.4","2.29.3"],"rank":6},{"purl":"pkg:npm/uuid@9.0.0","versions":["9.0.0","8.3.2"],"rank":6}]}]}`,
		},
		{
			name: "Single path with single component",
			input: HFHResultOutput{
				Results: []*HFHResult{
					{
						PathId: "src/lib",
						Components: []*HFHComponent{
							{
								Purl:     "pkg:npm/express@4.18.2",
								Versions: []string{"4.18.2"},
								Rank:     1,
							},
						},
					},
				},
			},
			expect: `{"results":[{"path_id":"src/lib","components":[{"purl":"pkg:npm/express@4.18.2","versions":["4.18.2"],"rank":1}]}]}`,
		},
		{
			name: "Path with no components",
			input: HFHResultOutput{
				Results: []*HFHResult{
					{
						PathId: "src/empty",
						// No asignamos Components, así que será nil y se omitirá en el JSON
					},
				},
			},
			expect: `{"results":[{"path_id":"src/empty"}]}`,
		},
		{
			name:   "Empty result set",
			input:  HFHResultOutput{},
			expect: `{}`,
		},
		{
			name: "Multiple components with same confidence",
			input: HFHResultOutput{
				Results: []*HFHResult{
					{
						PathId: "src/shared",
						Components: []*HFHComponent{
							{
								Purl:     "pkg:npm/typescript@5.0.4",
								Versions: []string{"5.0.4", "5.0.3"},
								Rank:     5,
							},
							{
								Purl:     "pkg:npm/prettier@2.8.7",
								Versions: []string{"2.8.7", "2.8.6"},
								Rank:     6,
							},
						},
					},
				},
			},
			expect: `{"results":[{"path_id":"src/shared","components":[{"purl":"pkg:npm/typescript@5.0.4","versions":["5.0.4","5.0.3"],"rank":5},{"purl":"pkg:npm/prettier@2.8.7","versions":["2.8.7","2.8.6"],"rank":6}]}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bytes, err := ExportHFHresult(s, tc.input)
			if err != nil {
				t.Errorf("Failed to export HFH result: %v\n", err)
			}

			// Compare the actual JSON output with expected
			if string(bytes) != tc.expect {
				t.Errorf("Export result mismatch.\nExpected: %s\nGot: %s", tc.expect, string(bytes))
			}

			// Verify the exported data can be parsed back to the original structure
			var parsed HFHResultOutput
			err = json.Unmarshal(bytes, &parsed)
			if err != nil {
				t.Errorf("Failed to parse exported data: %v\n", err)
			}
			if !cmp.Equal(tc.input, parsed) {
				t.Errorf("Export/Parse cycle failed.\nOriginal: %v\nGot: %v", tc.input, parsed)
			}

			fmt.Printf("Successfully tested %s: %v\n", tc.name, string(bytes))
		})
	}

	// Test special cases for empty or nil components
	specialCases := []struct {
		name  string
		input HFHResultOutput
	}{
		{
			name: "Nil components array",
			input: HFHResultOutput{
				Results: []*HFHResult{
					{
						PathId: "src/nil-components",
					},
				},
			},
		},
		{
			name: "Nil versions array",
			input: HFHResultOutput{
				Results: []*HFHResult{
					{
						PathId: "src/nil-versions",
						Components: []*HFHComponent{
							{
								Purl: "pkg:npm/test@1.0.0",
								Rank: 6,
							},
						},
					},
				},
			},
		},
	}

	for _, sc := range specialCases {
		t.Run(sc.name, func(t *testing.T) {
			bytes, err := ExportHFHresult(s, sc.input)
			if err != nil {
				t.Errorf("Failed to export special case: %v\n", err)
			}

			var parsed HFHResultOutput
			err = json.Unmarshal(bytes, &parsed)
			if err != nil {
				t.Errorf("Failed to parse special case data: %v\n", err)
			}
			if !cmp.Equal(sc.input, parsed) {
				t.Errorf("Special case Export/Parse cycle failed.\nOriginal: %v\nGot: %v", sc.input, parsed)
			}
		})
	}
}
