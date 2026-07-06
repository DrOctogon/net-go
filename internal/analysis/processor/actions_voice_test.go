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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/alerting"
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

// settingsWithKeywords builds enabled-transcription Settings with a keyword list.
func settingsWithKeywords(keywords []string, caseSensitive bool) *conf.Settings {
	s := settingsWithTranscription(true)
	s.Realtime.Transcription.Keywords = keywords
	s.Realtime.Transcription.KeywordCaseSensitive = caseSensitive
	return s
}

func TestTranscribeAction_KeywordHit_FlagsAndAlerts(t *testing.T) {
	// Not parallel: mutates the package-level alert bus singleton.
	bus := alerting.NewAlertEventBus(nil)
	t.Cleanup(bus.Stop)
	alerting.SetGlobalBus(bus)
	t.Cleanup(func() { alerting.SetGlobalBus(nil) })

	var got atomic.Pointer[alerting.AlertEvent]
	bus.Subscribe(func(e *alerting.AlertEvent) { got.Store(e) })

	transcriber := &fakeTranscriber{
		available: true,
		result:    transcription.Result{Text: "the house is on fire", Language: "en"},
	}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithKeywords([]string{"fire"}, false),
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))

	// Flag persisted exactly once with the matched keyword.
	require.Equal(t, 1, repo.GetKeywordFlagCalls(), "keyword hit must persist the flag once")
	id, flagged, hits := repo.GetLastKeywordFlag()
	assert.Equal(t, "42", id)
	assert.True(t, flagged)
	assert.Equal(t, "fire", hits)

	// Alert fired with the expected shape.
	require.Eventually(t, func() bool { return got.Load() != nil }, time.Second, 5*time.Millisecond)
	event := got.Load()
	assert.Equal(t, alerting.ObjectTypeKeywordFlag, event.ObjectType)
	assert.Equal(t, alerting.EventKeywordMatched, event.EventName)
	assert.Equal(t, "fire", event.Properties[alerting.PropertyKeywords])
	assert.Equal(t, uint(42), event.Properties[alerting.PropertyDetectionID])
	// Privacy default: the verbatim transcript must NOT be attached to the alert
	// (and thus never egressed to external channels) unless explicitly opted in.
	_, hasTranscript := event.Properties[alerting.PropertyTranscript]
	assert.False(t, hasTranscript, "transcript must be withheld from alerts by default")
}

func TestTranscribeAction_KeywordHit_IncludeTranscriptOptIn(t *testing.T) {
	// Not parallel: mutates the package-level alert bus singleton.
	bus := alerting.NewAlertEventBus(nil)
	t.Cleanup(bus.Stop)
	alerting.SetGlobalBus(bus)
	t.Cleanup(func() { alerting.SetGlobalBus(nil) })

	var got atomic.Pointer[alerting.AlertEvent]
	bus.Subscribe(func(e *alerting.AlertEvent) { got.Store(e) })

	transcriber := &fakeTranscriber{
		available: true,
		result:    transcription.Result{Text: "the house is on fire", Language: "en"},
	}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	settings := settingsWithKeywords([]string{"fire"}, false)
	settings.Realtime.Transcription.IncludeTranscriptInAlerts = true

	action := &TranscribeAction{
		Settings:     settings,
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))

	require.Eventually(t, func() bool { return got.Load() != nil }, time.Second, 5*time.Millisecond)
	event := got.Load()
	// Opt-in: the verbatim transcript is attached to the alert payload.
	assert.Equal(t, "the house is on fire", event.Properties[alerting.PropertyTranscript])
	assert.Equal(t, "fire", event.Properties[alerting.PropertyKeywords])
}

func TestTranscribeAction_NoKeywordHit_DoesNotFlagOrAlert(t *testing.T) {
	// Not parallel: mutates the package-level alert bus singleton.
	bus := alerting.NewAlertEventBus(nil)
	t.Cleanup(bus.Stop)
	alerting.SetGlobalBus(bus)
	t.Cleanup(func() { alerting.SetGlobalBus(nil) })

	var fired atomic.Int32
	bus.Subscribe(func(_ *alerting.AlertEvent) { fired.Add(1) })

	transcriber := &fakeTranscriber{
		available: true,
		result:    transcription.Result{Text: "everything is calm and quiet", Language: "en"},
	}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithKeywords([]string{"fire", "help"}, false),
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))

	// Transcript still persisted, but no keyword flag and no alert.
	assert.Equal(t, 1, repo.GetTranscriptCalls())
	assert.Equal(t, 0, repo.GetKeywordFlagCalls(), "no keyword match must not persist a flag")

	// Give the async bus a moment; it must remain silent.
	assert.Never(t, func() bool { return fired.Load() != 0 }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestTranscribeAction_EmptyKeywordList_IsNoOp(t *testing.T) {
	t.Parallel()

	transcriber := &fakeTranscriber{
		available: true,
		result:    transcription.Result{Text: "the house is on fire", Language: "en"},
	}
	repo := NewMockDetectionRepository()
	ctxData := &DetectionContext{}
	ctxData.NoteID.Store(42)

	action := &TranscribeAction{
		Settings:     settingsWithKeywords(nil, false), // no keywords configured
		Result:       testDetection().Result,
		Transcriber:  transcriber,
		Repo:         repo,
		DetectionCtx: ctxData,
		ClipName:     "clip.wav",
	}

	require.NoError(t, action.Execute(t.Context(), nil))
	assert.Equal(t, 0, repo.GetKeywordFlagCalls(), "empty keyword list must be a no-op")
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
