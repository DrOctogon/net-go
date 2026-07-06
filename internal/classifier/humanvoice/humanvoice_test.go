package humanvoice

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew_Errors verifies the constructor fails closed (wrapped error, nil
// model) when the config is nil or the Silero VAD model file is absent.
func TestNew_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
	}{
		{name: "nil config", cfg: nil},
		{
			name: "missing model file",
			cfg:  &Config{ModelPath: filepath.Join(t.TempDir(), "does_not_exist.onnx")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model, err := New(tt.cfg)
			require.Error(t, err)
			assert.Nil(t, model)
		})
	}
}

// TestAggregateSpeechProbability covers the pure max-over-frames aggregation,
// including empty input and out-of-range clamping.
func TestAggregateSpeechProbability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		frames []float32
		want   float32
	}{
		{name: "empty input yields zero", frames: nil, want: 0.0},
		{name: "single frame", frames: []float32{0.42}, want: 0.42},
		{name: "max over frames", frames: []float32{0.1, 0.9, 0.3, 0.85}, want: 0.9},
		{name: "all silence", frames: []float32{0.0, 0.0, 0.0}, want: 0.0},
		{name: "clamps above one", frames: []float32{0.2, 1.7}, want: 1.0},
		{name: "clamps below zero", frames: []float32{-0.5, -0.1}, want: 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := aggregateSpeechProbability(tt.frames)
			assert.InDelta(t, tt.want, got, 1e-6)
		})
	}
}

// TestAggregateClip verifies a single "Human Voice" result is produced with the
// max-confidence aggregation and the expected label fields.
func TestAggregateClip(t *testing.T) {
	t.Parallel()

	got := aggregateClip([]float32{0.2, 0.95, 0.4})

	assert.Equal(t, labelHumanVoice, got.Species)
	assert.Equal(t, labelHumanVoice, got.RawLabel)
	assert.InDelta(t, float32(0.95), got.Confidence, 1e-6)
}

// TestPredict_EmptyInput verifies that empty input produces no results and no
// error without requiring a loaded model.
func TestPredict_EmptyInput(t *testing.T) {
	t.Parallel()

	m := &Model{}
	results, err := m.Predict(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, results)

	results, err = m.Predict(t.Context(), [][]float32{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestModelMetadata verifies the static ModelInstance metadata (identity,
// labels, spec, runtime info) without loading a model binary.
func TestModelMetadata(t *testing.T) {
	t.Parallel()

	m := &Model{device: deviceCPU}

	assert.Equal(t, numSpeciesHumanVoice, m.NumSpecies())
	assert.Equal(t, []string{labelHumanVoice}, m.Labels())

	spec := m.Spec()
	assert.Equal(t, sampleRateHz, spec.SampleRate)
	assert.Equal(t, clipLength, spec.ClipLength)

	device, _, _ := m.RuntimeInfo()
	assert.Equal(t, deviceCPU, device)
}
