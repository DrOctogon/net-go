package humanvoice

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/inference"
)

// fakeVAD is a minimal inference.Classifier stand-in that lets Predict tests
// exercise the VAD-error and success paths without ONNX Runtime or a real
// Silero model.
type fakeVAD struct {
	predictFn func(samples []float32) ([]float32, error)
	closed    bool
}

var _ inference.Classifier = (*fakeVAD)(nil)

func (f *fakeVAD) Predict(samples []float32) ([]float32, error) { return f.predictFn(samples) }
func (f *fakeVAD) NumSpecies() int                              { return numSpeciesHumanVoice }
func (f *fakeVAD) Close()                                       { f.closed = true }

// errVADFailed is the sentinel the fake VAD returns to verify Predict
// propagates the underlying error rather than swallowing or panicking on it.
var errVADFailed = errors.New("vad backend exploded")

// TestPredict_VADError verifies that a VAD failure on any clip propagates as a
// wrapped error (no panic) and that no results are returned for that call.
func TestPredict_VADError(t *testing.T) {
	t.Parallel()

	m := &Model{vad: &fakeVAD{
		predictFn: func([]float32) ([]float32, error) { return nil, errVADFailed },
	}}

	results, err := m.Predict(t.Context(), [][]float32{{0.1, 0.2, 0.3}})

	require.ErrorIs(t, err, errVADFailed)
	assert.Nil(t, results)
}

// TestPredict_VADErrorOnLaterClip verifies that an error on a clip after the
// first still aborts the whole call without panicking, and that a preceding
// successful clip's result is discarded rather than partially returned.
func TestPredict_VADErrorOnLaterClip(t *testing.T) {
	t.Parallel()

	calls := 0
	m := &Model{vad: &fakeVAD{
		predictFn: func([]float32) ([]float32, error) {
			calls++
			if calls == 1 {
				return []float32{0.9}, nil
			}
			return nil, errVADFailed
		},
	}}

	results, err := m.Predict(t.Context(), [][]float32{{0.1}, {0.2}})

	require.ErrorIs(t, err, errVADFailed)
	assert.Nil(t, results)
}

// TestPredict_NilVAD verifies the uninitialized-model guard: a zero-value
// Model with no VAD backend fails closed with a descriptive error instead of
// panicking on a nil dereference, as long as there is at least one non-empty
// clip to process.
func TestPredict_NilVAD(t *testing.T) {
	t.Parallel()

	m := &Model{}

	results, err := m.Predict(t.Context(), [][]float32{{0.1, 0.2}})

	require.ErrorContains(t, err, "not initialized")
	assert.Nil(t, results)
}

// TestPredict_SkipsEmptyClips verifies that empty clips within a batch are
// skipped (no VAD call, no panic) while non-empty clips are still predicted.
func TestPredict_SkipsEmptyClips(t *testing.T) {
	t.Parallel()

	calls := 0
	m := &Model{vad: &fakeVAD{
		predictFn: func([]float32) ([]float32, error) {
			calls++
			return []float32{0.5}, nil
		},
	}}

	results, err := m.Predict(t.Context(), [][]float32{{}, {0.1, 0.2}, nil})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 1, calls)
	assert.Equal(t, labelHumanVoice, results[0].Species)
}

// TestPredict_Success verifies the happy path with a fake VAD: one aggregated
// result per non-empty clip, in order.
func TestPredict_Success(t *testing.T) {
	t.Parallel()

	m := &Model{vad: &fakeVAD{
		predictFn: func(samples []float32) ([]float32, error) {
			return []float32{samples[0]}, nil
		},
	}}

	results, err := m.Predict(t.Context(), [][]float32{{0.2}, {0.8}})

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.InDelta(t, float32(0.2), results[0].Confidence, 1e-6)
	assert.InDelta(t, float32(0.8), results[1].Confidence, 1e-6)
}
