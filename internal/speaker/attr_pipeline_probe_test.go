package speaker

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/inference"
)

// requireProbeORT skips the test unless both the committed probe fixture and the
// ONNX Runtime shared library are available. See TestGenderPipeline_SyntheticProbe
// for the rationale; the probes are SYNTHETIC (testdata/README.md), not real models.
func requireProbeORT(t *testing.T, fixture string) string {
	t.Helper()
	modelPath := filepath.Join("testdata", fixture)
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("probe fixture %q missing (regenerate with testdata/build_pipeline_probe.py): %v", fixture, err)
	}
	if err := inference.InitONNXRuntime(""); err != nil {
		t.Skipf("ONNX Runtime not available, skipping speaker pipeline probe: %v", err)
	}
	return modelPath
}

func constantClip(v float32) []float32 {
	const sampleCount = 16000 // one 1-second clip at the model's 16 kHz rate
	s := make([]float32, sampleCount)
	for i := range s {
		s[i] = v
	}
	return s
}

// TestAgePipeline_SyntheticProbe runs the age ONNX inference + mapping pipeline
// end to end against real ONNX Runtime using the synthetic age probe
// (score[batch,1] = clip mean). mapAge converts score*100 to years, so a
// mean of 0.05 -> 5y -> child and 0.35 -> 35y -> adult. Verifies input routing,
// the single-score output path, and the band + confidence mapping — not accuracy.
func TestAgePipeline_SyntheticProbe(t *testing.T) {
	modelPath := requireProbeORT(t, "pipeline_probe_age.onnx")

	analyzer, err := New(Config{Enabled: true, AgeEnabled: true, AgeModelPath: modelPath})
	require.NoError(t, err)
	if closer, ok := analyzer.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	cases := []struct {
		name string
		mean float32
		band string
	}{
		{"child", 0.05, AgeBandChild},
		{"adult", 0.35, AgeBandAdult},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attrs, err := analyzer.Analyze(t.Context(), [][]float32{constantClip(tc.mean)})
			require.NoError(t, err)
			assert.Equal(t, tc.band, attrs.AgeBand)
			assert.GreaterOrEqual(t, attrs.AgeConfidence, 0.0)
			assert.LessOrEqual(t, attrs.AgeConfidence, 1.0)
		})
	}
}

// TestVoicePrintPipeline_SyntheticProbe runs the voice-print ONNX inference
// pipeline end to end against real ONNX Runtime using the synthetic embedding
// probe (embedding[batch,8] = clip mean * [1..8]). Verifies input routing, the
// vector output path, that the embedding is surfaced, and that Cosine works on
// the extracted vector — not that the embedding is a meaningful speaker print.
func TestVoicePrintPipeline_SyntheticProbe(t *testing.T) {
	modelPath := requireProbeORT(t, "pipeline_probe_embedding.onnx")

	analyzer, err := New(Config{Enabled: true, VoicePrintEnabled: true, VoicePrintModelPath: modelPath})
	require.NoError(t, err)
	if closer, ok := analyzer.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	attrs, err := analyzer.Analyze(t.Context(), [][]float32{constantClip(0.5)})
	require.NoError(t, err)

	require.Len(t, attrs.Embedding, 8, "probe emits an 8-dim embedding")
	// mean 0.5 * [1..8] -> a non-zero vector; cosine with itself is 1.
	self := Cosine(attrs.Embedding, attrs.Embedding)
	assert.InDelta(t, 1.0, self, 1e-6, "self-cosine of a non-zero embedding is 1")
	assert.False(t, math.IsNaN(self))
}
