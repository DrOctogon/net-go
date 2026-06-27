package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResultHasSpeakerAttributes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		gender   string
		ageBand  string
		expected bool
	}{
		{"no attributes", "", "", false},
		{"gender only", "female", "", true},
		{"age band only", "", "adult", true},
		{"both", "male", "child", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := Result{Gender: tt.gender, AgeBand: tt.ageBand}
			assert.Equal(t, tt.expected, r.HasSpeakerAttributes())
		})
	}
}
