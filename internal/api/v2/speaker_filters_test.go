package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidSpeakerGenderFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid male", input: "male", want: "male"},
		{name: "valid female", input: "female", want: "female"},
		{name: "empty means no filter", input: "", want: ""},
		{name: "unrecognized value dropped", input: "robot", want: ""},
		{name: "case sensitive", input: "Male", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, validSpeakerGenderFilter(tt.input))
		})
	}
}

func TestValidSpeakerAgeBandFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid adult", input: "adult", want: "adult"},
		{name: "valid child", input: "child", want: "child"},
		{name: "empty means no filter", input: "", want: ""},
		{name: "unrecognized value dropped", input: "elder", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, validSpeakerAgeBandFilter(tt.input))
		})
	}
}

func TestValidSpeakerIDFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid cluster id", input: "spk_1", want: "spk_1"},
		{name: "valid multi-digit id", input: "spk_42", want: "spk_42"},
		{name: "empty means no filter", input: "", want: ""},
		{name: "missing prefix dropped", input: "42", want: ""},
		{name: "non-numeric suffix dropped", input: "spk_x", want: ""},
		{name: "bare prefix dropped", input: "spk_", want: ""},
		{name: "embedded whitespace dropped", input: "spk_1 ", want: ""},
		{name: "sql-ish garbage dropped", input: "spk_1;DROP TABLE notes", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, validSpeakerIDFilter(tt.input))
		})
	}
}
