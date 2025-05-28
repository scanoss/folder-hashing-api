package hfh_test

import (
	"reflect"
	"testing"

	"scanoss.com/hfh-api/pkg/hfh"
)

func TestHashToDenseVector(t *testing.T) {
	// Test cases based on the examples provided in comments
	tests := []struct {
		name        string
		hash        string
		expected    []float32
		expectError bool
	}{
		{
			name: "Hash cc7d3f78bd1dbb29",
			hash: "cc7d3f78bd1dbb29",
			expected: []float32{
				1, 1, 0, 0, 1, 1, 0, 0, 0, 1, 1, 1, 1, 1, 0, 1,
				0, 0, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, 1, 0, 0, 0,
				1, 0, 1, 1, 1, 1, 0, 1, 0, 0, 0, 1, 1, 1, 0, 1,
				1, 0, 1, 1, 1, 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 1,
			},
			expectError: false,
		},
		{
			name: "Hash f15c98316cfffcad",
			hash: "f15c98316cfffcad",
			expected: []float32{
				1, 1, 1, 1, 0, 0, 0, 1, 0, 1, 0, 1, 1, 1, 0, 0,
				1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 1,
				0, 1, 1, 0, 1, 1, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 0, 0, 1, 0, 1, 0, 1, 1, 0, 1,
			},
			expectError: false,
		},
		{
			name: "Hash 177fda3c2ce7bf1a",
			hash: "177fda3c2ce7bf1a",
			expected: []float32{
				0, 0, 0, 1, 0, 1, 1, 1, 0, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 0, 1, 1, 0, 1, 0, 0, 0, 1, 1, 1, 1, 0, 0,
				0, 0, 1, 0, 1, 1, 0, 0, 1, 1, 1, 0, 0, 1, 1, 1,
				1, 0, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 1, 0, 1, 0,
			},
			expectError: false,
		},
		{
			name:        "Empty hash string",
			hash:        "",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Invalid hex characters",
			hash:        "gg7d3f78bd1dbb29",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Hash too long for 64 bits",
			hash:        "1cc7d3f78bd1dbb29aa",
			expected:    nil,
			expectError: true,
		},
		{
			name: "All zeros",
			hash: "0000000000000000",
			expected: []float32{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			expectError: false,
		},
		{
			name: "All ones",
			hash: "ffffffffffffffff",
			expected: []float32{
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := hfh.HexSimhashToVector(tt.hash, hfh.VectorDim)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			// Check no error when not expected
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check result length
			if len(result) != hfh.VectorDim {
				t.Errorf("Expected vector length %d, got %d", hfh.VectorDim, len(result))
				return
			}

			// Check vector content
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Vector mismatch.\nExpected: %v\nGot:      %v", tt.expected, result)
			}
		})
	}
}
