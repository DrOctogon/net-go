// Package transcription turns saved voice clips into text. The default backend
// shells out to the whisper.cpp CLI (whisper-cli), mirroring how the rest of the
// project shells out to ffmpeg for audio work: it is optional, degrades
// gracefully when the binary or model is absent, and runs off the hot path.
package transcription

import "context"

// Result is the outcome of transcribing one clip.
type Result struct {
	// Text is the transcribed speech, trimmed. Empty when the clip contained no
	// recognizable speech (e.g. wind, silence).
	Text string
	// Language is the language the transcript was produced in (e.g. "en").
	Language string
	// DurationMs is the wall-clock time the transcription took.
	DurationMs int64
}

// Transcriber produces text from a saved audio clip.
type Transcriber interface {
	// Transcribe reads the WAV (or other ffmpeg-decodable) file at clipPath and
	// returns its transcript. Implementations must be safe to call concurrently.
	Transcribe(ctx context.Context, clipPath string) (Result, error)

	// Available reports whether the backend is usable right now (binary + model
	// present). Callers should skip transcription when this is false instead of
	// treating it as an error.
	Available() bool
}
