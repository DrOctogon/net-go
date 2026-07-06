package transcription

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWhisperCLI_Available covers the graceful-skip preconditions without
// invoking any external process.
func TestWhisperCLI_Available(t *testing.T) {
	t.Parallel()

	// No model configured -> not available.
	assert.False(t, NewWhisperCLI(Config{}).Available(), "empty model must be unavailable")

	// Model path that does not exist -> not available.
	assert.False(t, NewWhisperCLI(Config{Model: "/no/such/model.bin"}).Available(),
		"missing model file must be unavailable")
}

func TestNewWhisperCLI_Defaults(t *testing.T) {
	t.Parallel()
	w := NewWhisperCLI(Config{Model: "m.bin"})
	assert.Equal(t, "whisper-cli", w.cfg.Binary)
	assert.Equal(t, "ffmpeg", w.cfg.FFmpeg)
	assert.Equal(t, "en", w.cfg.Language)
}

// TestWhisperCLI_Transcribe runs a real transcription end-to-end. It is gated on
// the VOICEWATCH_TEST_WHISPER_MODEL and VOICEWATCH_TEST_WHISPER_AUDIO env vars
// (a GGML model path and an audio clip) so it stays skippable in environments
// without whisper.cpp installed.
func TestWhisperCLI_Transcribe(t *testing.T) {
	t.Parallel()

	model := os.Getenv("VOICEWATCH_TEST_WHISPER_MODEL")
	audio := os.Getenv("VOICEWATCH_TEST_WHISPER_AUDIO")
	if model == "" || audio == "" {
		t.Skip("set VOICEWATCH_TEST_WHISPER_MODEL and VOICEWATCH_TEST_WHISPER_AUDIO to run")
	}

	w := NewWhisperCLI(Config{Model: model})
	if !w.Available() {
		t.Skip("whisper-cli or ffmpeg not on PATH")
	}

	res, err := w.Transcribe(t.Context(), audio)
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(res.Text), "expected a non-empty transcript")
	assert.Equal(t, "en", res.Language)
	t.Logf("transcript: %q (%dms)", res.Text, res.DurationMs)
}
