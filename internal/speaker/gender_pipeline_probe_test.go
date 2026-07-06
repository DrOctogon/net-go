package speaker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/inference"
)

// TestGenderPipeline_SyntheticProbe runs the gender ONNX inference + mapping
// pipeline end to end against real ONNX Runtime using a committed synthetic
// probe model (testdata/pipeline_probe_2class.onnx).
//
// The probe is NOT a real gender classifier. It is a minimal
// `waveform[batch, samples] -> ReduceMean -> MatMul([1,-1]) -> logits[batch, 2]`
// graph whose only purpose is to exercise the real Go inference path with an
// actual .onnx file: SingleOutputAudioModel input routing, the [1, N] input
// tensor, the auto-allocated output tensor, and mapGender2Class. It projects the
// clip's mean amplitude to two class logits ([mean, -mean]) so the mapping's
// [male, female] order and the softmax confidence bound can be asserted
// deterministically.
//
// It skips when the ONNX Runtime shared library is not installed (e.g. a CI
// sandbox without `task download-onnxruntime`). For a REAL gender model and an
// accuracy check, use TestONNXGenderModelSmoke with VW_SPEAKER_GENDER_MODEL.
func TestGenderPipeline_SyntheticProbe(t *testing.T) {
	modelPath := filepath.Join("testdata", "pipeline_probe_2class.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("probe fixture missing (regenerate with testdata/build_pipeline_probe.py): %v", err)
	}
	if err := inference.InitONNXRuntime(""); err != nil {
		t.Skipf("ONNX Runtime not available, skipping speaker pipeline probe: %v", err)
	}

	analyzer, err := New(Config{Enabled: true, GenderEnabled: true, GenderModelPath: modelPath})
	require.NoError(t, err)
	if closer, ok := analyzer.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	const sampleCount = 16000 // one 1-second clip at the model's 16 kHz rate

	// Positive-mean waveform -> logits [+, -] -> argmax 0 (male index).
	t.Run("positive_mean_maps_to_male", func(t *testing.T) {
		samples := make([]float32, sampleCount)
		for i := range samples {
			samples[i] = 0.5
		}
		attrs, err := analyzer.Analyze(t.Context(), [][]float32{samples})
		require.NoError(t, err)
		assert.Equal(t, GenderMale, attrs.Gender)
		assert.Greater(t, attrs.GenderConfidence, 0.5)
		assert.LessOrEqual(t, attrs.GenderConfidence, 1.0)
	})

	// Negative-mean waveform -> logits [-, +] -> argmax 1 (female index).
	t.Run("negative_mean_maps_to_female", func(t *testing.T) {
		samples := make([]float32, sampleCount)
		for i := range samples {
			samples[i] = -0.5
		}
		attrs, err := analyzer.Analyze(t.Context(), [][]float32{samples})
		require.NoError(t, err)
		assert.Equal(t, GenderFemale, attrs.Gender)
	})
}
