package humanvoice_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/classifier/humanvoice"
	"github.com/tphakala/voicewatch/internal/inference"
)

// sampleCount is one 3-second clip at the model's 16 kHz native rate.
const sampleCount = 16000 * 3

// newTestModel extracts the embedded Silero VAD model and constructs a Model,
// skipping the test when the ONNX Runtime shared library is not installed
// (e.g. a CI sandbox without `task download-onnxruntime`).
func newTestModel(t *testing.T) *humanvoice.Model {
	t.Helper()
	modelPath, err := humanvoice.WriteEmbeddedModel(t.TempDir())
	require.NoError(t, err)

	if initErr := inference.InitONNXRuntime(""); initErr != nil {
		t.Skipf("ONNX Runtime not available, skipping Silero integration test: %v", initErr)
	}

	m, err := humanvoice.New(&humanvoice.Config{ModelPath: modelPath})
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// TestSilero_PredictSilence runs real inference on a silent clip and asserts the
// pipeline produces exactly one bounded "Human Voice" result.
func TestSilero_PredictSilence(t *testing.T) {
	m := newTestModel(t)

	silence := make([]float32, sampleCount) // all zeros
	results, err := m.Predict(context.Background(), [][]float32{silence})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "Human Voice", r.Species)
	assert.GreaterOrEqual(t, r.Confidence, float32(0))
	assert.LessOrEqual(t, r.Confidence, float32(1))
	// Pure silence should not read as confident speech.
	assert.Less(t, r.Confidence, float32(0.5), "silence should yield low speech confidence")
}

// TestSilero_PredictTone runs real inference on a synthetic tone and asserts a
// bounded result. (A tone is not speech; this exercises the run path and the
// [0,1] aggregation rather than detection accuracy.)
func TestSilero_PredictTone(t *testing.T) {
	m := newTestModel(t)

	tone := make([]float32, sampleCount)
	for i := range tone {
		tone[i] = 0.3 * float32(math.Sin(2*math.Pi*220*float64(i)/16000))
	}
	results, err := m.Predict(context.Background(), [][]float32{tone})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.GreaterOrEqual(t, results[0].Confidence, float32(0))
	assert.LessOrEqual(t, results[0].Confidence, float32(1))
}

// TestSilero_PredictEmpty returns no results for empty input.
func TestSilero_PredictEmpty(t *testing.T) {
	m := newTestModel(t)
	results, err := m.Predict(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, results)
}
