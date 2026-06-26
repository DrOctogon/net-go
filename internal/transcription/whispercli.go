package transcription

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tphakala/voicewatch/internal/errors"
)

// whisperSampleRate is the sample rate whisper.cpp requires (16 kHz mono).
const whisperSampleRate = "16000"

// Config configures the whisper.cpp CLI transcription backend.
type Config struct {
	// Binary is the whisper.cpp CLI executable (name on PATH or absolute path).
	Binary string
	// Model is the path to the GGML model file (e.g. ggml-tiny.en.bin). Required.
	Model string
	// FFmpeg is the ffmpeg executable used to resample clips to 16 kHz mono.
	FFmpeg string
	// Language is the spoken language hint passed to whisper (e.g. "en", "auto").
	Language string
}

// WhisperCLI transcribes clips by resampling them to 16 kHz mono with ffmpeg and
// running the whisper.cpp CLI. It is safe for concurrent use: each call writes to
// its own temporary file and spawns its own processes.
type WhisperCLI struct {
	cfg Config
}

// compile-time assertion that WhisperCLI implements Transcriber.
var _ Transcriber = (*WhisperCLI)(nil)

// NewWhisperCLI builds a whisper.cpp CLI transcriber, filling in sensible
// defaults for any unset field.
func NewWhisperCLI(cfg Config) *WhisperCLI {
	if cfg.Binary == "" {
		cfg.Binary = "whisper-cli"
	}
	if cfg.FFmpeg == "" {
		cfg.FFmpeg = "ffmpeg"
	}
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	return &WhisperCLI{cfg: cfg}
}

// Available reports whether the whisper CLI, ffmpeg, and the model file are all
// present, so callers can skip transcription cleanly when it is not configured.
func (w *WhisperCLI) Available() bool {
	if w.cfg.Model == "" {
		return false
	}
	if _, err := os.Stat(w.cfg.Model); err != nil {
		return false
	}
	if _, err := exec.LookPath(w.cfg.Binary); err != nil {
		return false
	}
	if _, err := exec.LookPath(w.cfg.FFmpeg); err != nil {
		return false
	}
	return true
}

// Transcribe resamples clipPath to 16 kHz mono and runs whisper.cpp over it,
// returning the trimmed transcript. A clip with no recognizable speech yields an
// empty Text and no error.
func (w *WhisperCLI) Transcribe(ctx context.Context, clipPath string) (Result, error) {
	start := time.Now()

	if clipPath == "" {
		return Result{}, errors.Newf("transcription: empty clip path").
			Category(errors.CategoryValidation).
			Build()
	}
	if w.cfg.Model == "" {
		return Result{}, errors.Newf("transcription: no model configured").
			Category(errors.CategoryConfiguration).
			Build()
	}

	wav16k, cleanup, err := w.resampleTo16k(ctx, clipPath)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	text, err := w.runWhisper(ctx, wav16k)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Text:       text,
		Language:   w.cfg.Language,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// resampleTo16k converts clipPath to a temporary 16 kHz mono WAV that whisper
// requires, returning the temp path and a cleanup func.
func (w *WhisperCLI) resampleTo16k(ctx context.Context, clipPath string) (path string, cleanup func(), err error) {
	tmp, err := os.CreateTemp("", "voicewatch-stt-*.wav")
	if err != nil {
		return "", func() {}, errors.New(err).
			Category(errors.CategoryFileIO).
			Context("operation", "create_temp_wav").
			Build()
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	cleanup = func() { _ = os.Remove(tmpPath) }

	// -y overwrite, -ar 16000, -ac 1 mono; output is the temp wav.
	cmd := exec.CommandContext(ctx, w.cfg.FFmpeg,
		"-y", "-i", clipPath, "-ar", whisperSampleRate, "-ac", "1", tmpPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		cleanup()
		return "", func() {}, errors.New(runErr).
			Category(errors.CategoryAudio).
			Context("operation", "ffmpeg_resample").
			Context("clip", clipPath).
			Build()
	}
	return tmpPath, cleanup, nil
}

// runWhisper executes whisper-cli over a 16 kHz WAV and returns the trimmed
// transcript. -nt drops timestamps, -np drops progress prints, so stdout is the
// transcript text only.
func (w *WhisperCLI) runWhisper(ctx context.Context, wav16k string) (string, error) {
	cmd := exec.CommandContext(ctx, w.cfg.Binary,
		"-m", w.cfg.Model,
		"-f", wav16k,
		"-l", w.cfg.Language,
		"-nt", // no timestamps
		"-np", // no progress / no extra prints
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.New(err).
			Category(errors.CategoryProcessing).
			Context("operation", "whisper_cli").
			Context("model", w.cfg.Model).
			Build()
	}
	return strings.TrimSpace(stdout.String()), nil
}
