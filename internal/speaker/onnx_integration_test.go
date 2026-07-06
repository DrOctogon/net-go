package speaker

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/tphakala/voicewatch/internal/inference/onnx"
)

// TestONNXGenderModelSmoke is the turn-key validation harness for a real gender
// model, intended to be run inside an ORT-enabled container (the ONNX Runtime
// shared library and the .onnx file are absent from the normal build/test env,
// so this test SKIPS unless both are provided).
//
// Run it like:
//
//	VW_ORT_LIB=/path/to/libonnxruntime.so \
//	VW_SPEAKER_GENDER_MODEL=/path/to/voice-gender.onnx \
//	[VW_SPEAKER_TEST_PCM=/path/to/clip.f32le]  # 16 kHz mono float32 LE; else synthetic tone \
//	[VW_SPEAKER_EXPECT=male|female]            # assert the predicted label \
//	go test -run TestONNXGenderModelSmoke -tags "notflite skipfrontend" ./internal/speaker/
//
// Without VW_SPEAKER_TEST_PCM it feeds a synthetic tone — that only proves the
// load → preprocess → infer → map pipeline works end to end (a smoke test). For
// an accuracy check, supply real labeled speech via VW_SPEAKER_TEST_PCM +
// VW_SPEAKER_EXPECT, ideally across several male and female clips.
func TestONNXGenderModelSmoke(t *testing.T) {
	libPath := os.Getenv("VW_ORT_LIB")
	modelPath := os.Getenv("VW_SPEAKER_GENDER_MODEL")
	if libPath == "" || modelPath == "" {
		t.Skip("set VW_ORT_LIB and VW_SPEAKER_GENDER_MODEL to run the real-model harness")
	}

	onnx.MustInitORT(libPath)
	t.Cleanup(func() { _ = onnx.DestroyORT() })

	analyzer, err := New(Config{
		Enabled:         true,
		GenderEnabled:   true,
		GenderModelPath: modelPath,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if closer, ok := analyzer.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	samples := loadTestPCM(t)
	attrs, err := analyzer.Analyze(t.Context(), [][]float32{samples})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	t.Logf("gender=%q confidence=%.4f", attrs.Gender, attrs.GenderConfidence)

	if !IsValidGender(attrs.Gender) || attrs.Gender == GenderUnknown {
		t.Fatalf("expected a concrete gender (male/female), got %q", attrs.Gender)
	}
	if attrs.GenderConfidence < 0 || attrs.GenderConfidence > 1 {
		t.Fatalf("confidence out of range: %v", attrs.GenderConfidence)
	}
	if want := os.Getenv("VW_SPEAKER_EXPECT"); want != "" && attrs.Gender != want {
		t.Errorf("predicted gender %q, expected %q", attrs.Gender, want)
	}
}

// loadTestPCM returns 16 kHz mono float32 samples from VW_SPEAKER_TEST_PCM
// (raw little-endian float32) when set, otherwise a 3-second synthetic tone.
func loadTestPCM(t *testing.T) []float32 {
	t.Helper()
	if path := os.Getenv("VW_SPEAKER_TEST_PCM"); path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied test fixture path
		if err != nil {
			t.Fatalf("read VW_SPEAKER_TEST_PCM: %v", err)
		}
		n := len(raw) / 4
		out := make([]float32, n)
		for i := range out {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
		return out
	}

	const sampleRate = 16000
	const seconds = 3
	const freq = 150.0 // a low tone in the human-voice F0 range
	out := make([]float32, sampleRate*seconds)
	for i := range out {
		out[i] = float32(0.2 * math.Sin(2*math.Pi*freq*float64(i)/sampleRate))
	}
	return out
}
