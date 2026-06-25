package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tphakala/voicewatch/internal/app"
	"github.com/tphakala/voicewatch/internal/conf"
)

// Compile-time interface compliance check.
var _ app.Analyzer = (*VoiceWatchAnalyzer)(nil)

func TestVoiceWatchAnalyzer_Name(t *testing.T) {
	t.Parallel()

	a := NewVoiceWatchAnalyzer(&conf.Settings{})
	assert.Equal(t, "voicewatch-analyzer", a.Name())
}

func TestVoiceWatchAnalyzer_Compatible(t *testing.T) {
	t.Parallel()

	a := NewVoiceWatchAnalyzer(&conf.Settings{})

	tests := []struct {
		name       string
		sourceType app.SourceType
		want       bool
	}{
		{"audio card is compatible", app.SourceTypeAudioCard, true},
		{"RTSP is compatible", app.SourceTypeRTSP, true},
		{"ultrasonic is not compatible", app.SourceTypeUltrasonic, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := app.AudioSource{Type: tt.sourceType}
			assert.Equal(t, tt.want, a.Compatible(src))
		})
	}
}

func TestVoiceWatchAnalyzer_BirdNET_NilBeforeStart(t *testing.T) {
	t.Parallel()

	a := NewVoiceWatchAnalyzer(&conf.Settings{})
	assert.Nil(t, a.Orchestrator(), "Orchestrator() should return nil before Start()")
}

func TestVoiceWatchAnalyzer_Stop_NilSafe(t *testing.T) {
	t.Parallel()

	a := NewVoiceWatchAnalyzer(&conf.Settings{})
	// Stop before Start should not panic and should return nil.
	assert.NotPanics(t, func() {
		err := a.Stop(t.Context())
		assert.NoError(t, err)
	})
}
