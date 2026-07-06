package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeepMergeMaps tests the deep merge functionality
func TestDeepMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]any
		src      map[string]any
		expected map[string]any
	}{
		{
			name: "Simple merge",
			dst: map[string]any{
				"a": 1,
				"b": 2,
			},
			src: map[string]any{
				"b": 3,
				"c": 4,
			},
			expected: map[string]any{
				"a": 1,
				"b": 3,
				"c": 4,
			},
		},
		{
			name: "Nested merge",
			dst: map[string]any{
				"top": map[string]any{
					"a": 1,
					"b": 2,
				},
			},
			src: map[string]any{
				"top": map[string]any{
					"b": 3,
					"c": 4,
				},
			},
			expected: map[string]any{
				"top": map[string]any{
					"a": 1,
					"b": 3,
					"c": 4,
				},
			},
		},
		{
			name: "Preserve unmodified nested objects",
			dst: map[string]any{
				"settings": map[string]any{
					"rangeFilter": map[string]any{
						"model":     "latest",
						"threshold": 0.03,
					},
					"latitude": 40.0,
				},
			},
			src: map[string]any{
				"settings": map[string]any{
					"latitude": 50.0,
				},
			},
			expected: map[string]any{
				"settings": map[string]any{
					"rangeFilter": map[string]any{
						"model":     "latest",
						"threshold": 0.03,
					},
					"latitude": 50.0,
				},
			},
		},
		{
			name: "Handle nil values",
			dst: map[string]any{
				"a": 1,
				"b": 2,
			},
			src: map[string]any{
				"b": nil,
				"c": 3,
			},
			expected: map[string]any{
				"a": 1,
				"b": nil,
				"c": 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deepMergeMaps(tt.dst, tt.src)
			assert.Equal(t, tt.expected, result)
		})
	}
}
