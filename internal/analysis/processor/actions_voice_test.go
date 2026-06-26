// actions_voice_test.go - Tests for TranscribeAction.Execute()
//
// These tests verify the transcriber gating contract WITHOUT requiring whisper-cli
// or ffmpeg to be installed. A fake Transcriber stands in for the whisper.cpp CLI
// backend so the tests exercise the action's control flow only:
//   - no-op when transcription is disabled (hot-reload safe)
//   - no-op when the backend is unavailable
//   - transcribe + persist + share via DetectionContext on the happy path
//   - retryable error when the detection ID is not yet assigned
package processor

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/transcription"
)

// fakeTranscriber is a test double for transcription.Transcriber. It records
// whether Transcribe was called and never shells out to any external process.
type fakeTranscriber struct {
	available   bool
	result      transcription.Result
	err         error
	transcribed atomic.Int32
}

func (f *fakeTranscriber) Available() bool { return f.available }

func (f *fakeTranscriber) Transcribe(_ context.Context, _ string) (transcription.Result, error) {
	f.transcribed.Add(1)
	if f.err != nil {
		return transcription.Result{}, f.err
	}
	return f.result, nil
}

// settingsWithTranscription builds Settings with transcription toggled as given.
func settingsWithTranscription(enabled bool) *conf.Settings {
	s := &conf.Settings{Debug: true}
	s.Realtime.Transcription.Enabled = enabled
	s.Realtime.Audio.Export.Path = "clips"
	return s
}

func TestTranscribeAction_Disabled_IsNoOp(t *testing.T) {
	t.Parallel()

	transcriber := &fakeTranscriber{available: true, result: transcription.Result{Text: "hello", Language: "en"}}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithTranscription(false), // disabled
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))
	assert.Equal(t, int32(0), transcriber.transcribed.Load(), "disabled transcription must not invoke the backend")
	assert.Equal(t, 0, repo.GetTranscriptCalls(), "disabled transcription must not persist")
	assert.Nil(t, ctxData.Transcript(), "disabled transcription must not populate the context")
}

func TestTranscribeAction_Unavailable_IsNoOp(t *testing.T) {
	t.Parallel()

	transcriber := &fakeTranscriber{available: false} // binary/model missing
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithTranscription(true),
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil), "unavailable backend must skip cleanly, not error")
	assert.Equal(t, int32(0), transcriber.transcribed.Load(), "unavailable backend must not be invoked")
	assert.Equal(t, 0, repo.GetTranscriptCalls(), "unavailable backend must not persist")
}

func TestTranscribeAction_NilTranscriber_IsNoOp(t *testing.T) {
	t.Parallel()

	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithTranscription(true),
		Result:       testDetection().Result,
		Transcriber:  nil, // not configured
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))
	assert.Equal(t, 0, repo.GetTranscriptCalls())
}

func TestTranscribeAction_HappyPath_PersistsAndSharesTranscript(t *testing.T) {
	t.Parallel()

	transcriber := &fakeTranscriber{
		available: true,
		result:    transcription.Result{Text: "hello there", Language: "en", DurationMs: 12},
	}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithTranscription(true),
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))

	assert.Equal(t, int32(1), transcriber.transcribed.Load(), "backend should be invoked once")

	// Persisted via the repository with the detection ID from the context.
	require.Equal(t, 1, repo.GetTranscriptCalls(), "transcript should be persisted once")
	id, text, lang := repo.GetLastTranscript()
	assert.Equal(t, "42", id, "persisted under the database-assigned detection ID")
	assert.Equal(t, "hello there", text)
	assert.Equal(t, "en", lang)

	// Shared via the detection context for late consumers.
	shared := ctxData.Transcript()
	require.NotNil(t, shared, "transcript should be shared via the context")
	assert.Equal(t, "hello there", shared.Text)
	assert.Equal(t, "en", shared.Language)
}

func TestTranscribeAction_NoDetectionID_ReturnsRetryableError(t *testing.T) {
	t.Parallel()

	transcriber := &fakeTranscriber{available: true, result: transcription.Result{Text: "hello", Language: "en"}}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{} // NoteID stays 0 (DatabaseAction hasn't run yet)

	action := &TranscribeAction{
		Settings:     settingsWithTranscription(true),
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	err := action.Execute(t.Context(), nil)
	require.Error(t, err, "missing detection ID should produce a (retryable) error so the job retries")
	assert.Equal(t, int32(0), transcriber.transcribed.Load(), "must not transcribe before the detection ID is available")
	assert.Equal(t, 0, repo.GetTranscriptCalls(), "must not persist without a detection ID")
}
