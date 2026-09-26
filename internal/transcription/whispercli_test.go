package transcription

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/errors"
)

// testModelPath is a placeholder GGML model path for tests that never read the
// model file: Transcribe only checks Config.Model is non-empty, and Available()
// (which does stat it) is covered separately.
const testModelPath = "m.bin"

// writeFakeBin writes a POSIX shell script to dir/name and makes it executable,
// so tests can stand in for the real ffmpeg/whisper-cli binaries without
// depending on either being installed. POSIX-only; callers must skipOnWindows.
func writeFakeBin(t *testing.T, dir, name, script string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755)) //nolint:gosec // test-only fake binary must be executable
	return p
}

// skipOnWindows skips tests that rely on POSIX shell-script fake binaries;
// CI's main coverage runner is linux.
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == winGOOS {
		t.Skip("fake shell-script binaries are POSIX-only")
	}
}

// winGOOS is runtime.GOOS's value on Windows.
const winGOOS = "windows"

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
	w := NewWhisperCLI(Config{Model: testModelPath})
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

// TestWhisperCLI_Transcribe_FFmpegFailure covers the resample failure path with
// a fake "ffmpeg" that always exits non-zero, so it needs neither real ffmpeg
// nor whisper-cli. whisper-cli is never reached: Binary is intentionally left
// unset/bogus to prove the failure happens at the resample stage.
func TestWhisperCLI_Transcribe_FFmpegFailure(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	failFFmpeg := writeFakeBin(t, dir, "ffmpeg-fail.sh", "#!/bin/sh\necho boom >&2\nexit 1\n")

	w := NewWhisperCLI(Config{Model: testModelPath, FFmpeg: failFFmpeg, Binary: "/does/not/exist"})

	res, err := w.Transcribe(t.Context(), "clip.wav")
	require.Error(t, err)
	assert.Equal(t, Result{}, res, "no partial result on failure")

	var enhanced *errors.EnhancedError
	require.ErrorAs(t, err, &enhanced, "resample failure must carry internal/errors telemetry metadata")
	assert.Equal(t, errors.CategoryAudio, enhanced.Category, "ffmpeg failures categorize as audio")
	assert.Equal(t, "ffmpeg_resample", enhanced.GetContext()["operation"], "operation context")
	assert.Equal(t, "clip.wav", enhanced.GetContext()["clip"], "clip context")
}

// TestWhisperCLI_Transcribe_WhisperFailure covers the transcription failure
// path: ffmpeg "succeeds" (a no-op fake binary) but whisper-cli always exits
// non-zero. Needs neither real ffmpeg nor whisper-cli.
func TestWhisperCLI_Transcribe_WhisperFailure(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Drains stdin (none here) and exits 0 without touching the output path;
	// resampleTo16k only cares that the command succeeded, not the file content.
	okFFmpeg := writeFakeBin(t, dir, "ffmpeg-ok.sh", "#!/bin/sh\nexit 0\n")
	failWhisper := writeFakeBin(t, dir, "whisper-fail.sh", "#!/bin/sh\necho garbled >&2\nexit 2\n")

	w := NewWhisperCLI(Config{Model: testModelPath, FFmpeg: okFFmpeg, Binary: failWhisper})

	res, err := w.Transcribe(t.Context(), "clip.wav")
	require.Error(t, err)
	assert.Equal(t, Result{}, res, "no partial result on failure")

	var enhanced *errors.EnhancedError
	require.ErrorAs(t, err, &enhanced, "whisper failure must carry internal/errors telemetry metadata")
	assert.Equal(t, errors.CategoryProcessing, enhanced.Category, "whisper failures categorize as processing")
	assert.Equal(t, "whisper_cli", enhanced.GetContext()["operation"], "operation context")
	assert.Equal(t, testModelPath, enhanced.GetContext()["model"], "model context")
}

// TestWhisperCLI_resampleTo16k_ContextCancellation covers cancellation mid-run:
// a fake "ffmpeg" that sleeps far longer than the test's patience, cancelled
// almost immediately. Asserts the call returns promptly (proving the process
// was actually killed, not merely that Run() eventually returned) and produces
// a categorized error rather than hanging or panicking.
func TestWhisperCLI_resampleTo16k_ContextCancellation(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	sleepyFFmpeg := writeFakeBin(t, dir, "ffmpeg-sleep.sh", "#!/bin/sh\nsleep 5\nexit 0\n")

	w := NewWhisperCLI(Config{Model: testModelPath, FFmpeg: sleepyFFmpeg})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, cleanup, err := w.resampleTo16k(ctx, "clip.wav")
	elapsed := time.Since(start)
	cleanup()

	require.Error(t, err)
	assert.Less(t, elapsed, 3*time.Second,
		"context cancellation must kill the process well before its 5s sleep completes")

	var enhanced *errors.EnhancedError
	require.ErrorAs(t, err, &enhanced, "cancelled resample must still carry internal/errors telemetry metadata")
	assert.Equal(t, errors.CategoryAudio, enhanced.Category, "ffmpeg failures categorize as audio")
	assert.Equal(t, "ffmpeg_resample", enhanced.GetContext()["operation"], "operation context")
}
