package onnx

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ort "github.com/yalue/onnxruntime_go"
)

// floatInfo builds an InputOutputInfo with the given name and a float32 element
// type; nonFloatInfo builds one with a non-float (int64) element type. Only the
// Name and DataType fields are read by the routing helpers.
func floatInfo(name string) ort.InputOutputInfo {
	return ort.InputOutputInfo{Name: name, DataType: ort.TensorElementDataTypeFloat}
}

func nonFloatInfo(name string) ort.InputOutputInfo {
	return ort.InputOutputInfo{Name: name, DataType: ort.TensorElementDataTypeInt64}
}

// routeCase is one shared test case for the single-float routing helpers.
type routeCase struct {
	name     string
	infos    []ort.InputOutputInfo
	wantName string
	wantErr  bool
}

// routeCases is shared by both routeAudioInput and routeSingleFloatOutput: both
// require exactly one float32 tensor and otherwise return a SpeakerModelIOError.
func routeCases() []routeCase {
	return []routeCase{
		{"single-float", []ort.InputOutputInfo{floatInfo("x")}, "x", false},
		{"float-plus-nonfloat", []ort.InputOutputInfo{floatInfo("x"), nonFloatInfo("aux")}, "x", false},
		{"no-float", []ort.InputOutputInfo{nonFloatInfo("aux")}, "", true},
		{"empty", nil, "", true},
		{"two-floats", []ort.InputOutputInfo{floatInfo("a"), floatInfo("b")}, "", true},
	}
}

// runRouteTest exercises a routing helper against the shared cases.
func runRouteTest(t *testing.T, route func([]ort.InputOutputInfo) (string, error)) {
	t.Helper()
	for _, tt := range routeCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, err := route(tt.infos)
			if tt.wantErr {
				var ioErr *SpeakerModelIOError
				require.ErrorAs(t, err, &ioErr, "must be a SpeakerModelIOError")
				assert.Empty(t, name)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

func TestRouteAudioInput(t *testing.T) {
	t.Parallel()
	runRouteTest(t, routeAudioInput)
}

func TestRouteSingleFloatOutput(t *testing.T) {
	t.Parallel()
	runRouteTest(t, routeSingleFloatOutput)
}

func TestNormalizeWaveform(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, normalizeWaveform(nil))
	})

	t.Run("constant-is-zeroed", func(t *testing.T) {
		t.Parallel()
		// A constant signal has zero variance; (x-mean)=0 so every output is 0.
		out := normalizeWaveform([]float32{0.3, 0.3, 0.3, 0.3})
		require.Len(t, out, 4)
		for _, v := range out {
			assert.InDelta(t, 0.0, float64(v), 1e-6)
		}
	})

	t.Run("zero-mean-unit-variance", func(t *testing.T) {
		t.Parallel()
		in := []float32{1, 2, 3, 4, 5, 6, 7, 8}
		out := normalizeWaveform(in)
		require.Len(t, out, len(in))

		var sum float64
		for _, v := range out {
			sum += float64(v)
		}
		mean := sum / float64(len(out))
		assert.InDelta(t, 0.0, mean, 1e-6, "normalized mean ~ 0")

		var varSum float64
		for _, v := range out {
			d := float64(v) - mean
			varSum += d * d
		}
		variance := varSum / float64(len(out))
		// eps makes the result slightly below 1; for a sizeable signal it is ~1.
		assert.InDelta(t, 1.0, variance, 1e-3, "normalized variance ~ 1")
	})

	t.Run("does-not-mutate-input", func(t *testing.T) {
		t.Parallel()
		in := []float32{1, 2, 3}
		_ = normalizeWaveform(in)
		assert.Equal(t, []float32{1, 2, 3}, in)
	})

	t.Run("no-nan-on-tiny-signal", func(t *testing.T) {
		t.Parallel()
		// Near-zero variance must not divide-by-zero thanks to the eps term.
		out := normalizeWaveform([]float32{1e-9, 1e-9, 1e-9})
		for _, v := range out {
			assert.False(t, math.IsNaN(float64(v)) || math.IsInf(float64(v), 0))
		}
	})
}
