// keywords_test.go - Tests for transcript keyword matching.
package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		transcript    string
		keywords      []string
		caseSensitive bool
		want          []string
	}{
		{
			name:       "empty keyword list is a no-op",
			transcript: "the house is on fire",
			keywords:   nil,
			want:       nil,
		},
		{
			name:       "empty transcript yields no hits",
			transcript: "",
			keywords:   []string{"fire"},
			want:       nil,
		},
		{
			name:       "single hit case-insensitive by default",
			transcript: "The house is on FIRE now",
			keywords:   []string{"fire"},
			want:       []string{"fire"},
		},
		{
			name:       "no hit when keyword absent",
			transcript: "everything is calm and quiet",
			keywords:   []string{"fire", "help"},
			want:       nil,
		},
		{
			name:          "case sensitive miss",
			transcript:    "the house is on FIRE",
			keywords:      []string{"fire"},
			caseSensitive: true,
			want:          nil,
		},
		{
			name:          "case sensitive hit",
			transcript:    "the house is on fire",
			keywords:      []string{"fire"},
			caseSensitive: true,
			want:          []string{"fire"},
		},
		{
			name:       "word boundary prevents substring match",
			transcript: "I love firefighters and campfires",
			keywords:   []string{"fire"},
			want:       nil,
		},
		{
			name:       "word boundary matches whole word",
			transcript: "there is a fire over there",
			keywords:   []string{"fire"},
			want:       []string{"fire"},
		},
		{
			name:       "multiple hits preserve config order",
			transcript: "call for help, there is a fire",
			keywords:   []string{"fire", "help"},
			want:       []string{"fire", "help"},
		},
		{
			name:       "multi-word phrase match",
			transcript: "please call nine one one right away",
			keywords:   []string{"nine one one"},
			want:       []string{"nine one one"},
		},
		{
			name:       "duplicate keywords deduplicated case-insensitively",
			transcript: "fire fire everywhere",
			keywords:   []string{"fire", "Fire"},
			want:       []string{"fire"},
		},
		{
			name:       "blank keyword entries are skipped",
			transcript: "anything goes here",
			keywords:   []string{"   ", ""},
			want:       nil,
		},
		{
			name:       "regex metacharacters treated literally",
			transcript: "please dial 9-1-1 immediately",
			keywords:   []string{"9-1-1"},
			want:       []string{"9-1-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchKeywords(tt.transcript, tt.keywords, tt.caseSensitive)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJoinKeywords(t *testing.T) {
	t.Parallel()
	assert.Empty(t, joinKeywords(nil))
	assert.Equal(t, "fire", joinKeywords([]string{"fire"}))
	assert.Equal(t, "fire,help", joinKeywords([]string{"fire", "help"}))
}
