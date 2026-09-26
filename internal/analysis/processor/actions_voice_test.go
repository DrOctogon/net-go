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
	"github.com/tphakala/voicewatch/internal/analysis/jobqueue"
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

// concurrencyTrackingTranscriber is a Transcriber test double that simulates a
// heavy whisper-cli invocation: it sleeps for `delay` per call and records the
// highest number of calls it ever saw in flight simultaneously. Used to prove
// transcribeSem (internal/analysis/processor/actions_voice.go) actually
// serializes transcription work.
type concurrencyTrackingTranscriber struct {
	delay   time.Duration
	current atomic.Int32
	max     atomic.Int32
}

func (c *concurrencyTrackingTranscriber) Available() bool { return true }

func (c *concurrencyTrackingTranscriber) Transcribe(_ context.Context, _ string) (transcription.Result, error) {
	n := c.current.Add(1)
	defer c.current.Add(-1)
	for {
		observedMax := c.max.Load()
		if n <= observedMax {
			break
		}
		if c.max.CompareAndSwap(observedMax, n) {
			break
		}
	}
	time.Sleep(c.delay)
	return transcription.Result{Text: "hi", Language: "en"}, nil
}

// instantAction is a minimal jobqueue.Action stand-in for an unrelated action
// type (e.g. SSE/MQTT/database). It reports the time it ran so the test can
// prove it was not queued behind the transcription jobs.
type instantAction struct {
	ranAt chan time.Time
}

func (a *instantAction) Execute(_ context.Context, _ any) error {
	a.ranAt <- time.Now()
	return nil
}

func (a *instantAction) GetDescription() string { return "instant test action" }

// TestTranscribeAction_ConcurrencyCap_SerializesAcrossJobs drives real
// TranscribeAction jobs through a real jobqueue.JobQueue (mirroring how the
// processor wires them up) to prove two things about the maxConcurrentTranscriptions
// cap: (1) three concurrently-due transcription jobs never run their backend at
// the same time, and (2) an unrelated action type queued alongside them is not
// blocked behind the cap - the job queue still spawns its goroutine immediately.
func TestTranscribeAction_ConcurrencyCap_SerializesAcrossJobs(t *testing.T) {
	// Not parallel: transcribeSem is package-level, so a sibling t.Parallel()
	// test also calling TranscribeAction.Execute would add noise to the timing
	// assertions below.
	const (
		numTranscriptions = 3
		simulatedWork     = 80 * time.Millisecond
	)

	ctx := t.Context()
	queue := jobqueue.NewJobQueueWithOptions(10, false)
	queue.SetProcessingInterval(5 * time.Millisecond)
	queue.Start()
	t.Cleanup(func() {
		assert.NoError(t, queue.StopWithTimeout(5*time.Second))
	})

	tracker := &concurrencyTrackingTranscriber{delay: simulatedWork}
	repo := NewMockDetectionRepository()

	start := time.Now()

	for i := range numTranscriptions {
		ctxData := &DetectionContext{}
		ctxData.NoteID.Store(uint64(i + 1))
		action := &TranscribeAction{
			Settings:     settingsWithTranscription(true),
			Result:       testDetection().Result,
			Transcriber:  tracker,
			Repo:         repo,
			DetectionCtx: ctxData,
			ClipName:     "clip.wav",
		}
		_, err := queue.Enqueue(ctx, action, nil, jobqueue.RetryConfig{Enabled: false})
		require.NoError(t, err)
	}

	fast := &instantAction{ranAt: make(chan time.Time, 1)}
	_, err := queue.Enqueue(ctx, fast, nil, jobqueue.RetryConfig{Enabled: false})
	require.NoError(t, err)

	var fastRanAt time.Time
	select {
	case fastRanAt = <-fast.ranAt:
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated action never ran - it must not queue behind transcriptions")
	}

	require.Eventually(t, func() bool {
		return queue.GetStats().SuccessfulJobs == numTranscriptions+1
	}, 5*time.Second, 5*time.Millisecond, "all transcription jobs should eventually succeed")

	// The unrelated action type ran well before three serialized transcriptions
	// (each simulatedWork long) could possibly have finished - proving the cap
	// lives inside TranscribeAction.Execute, not in the queue's dispatcher.
	assert.Less(t, fastRanAt.Sub(start), simulatedWork,
		"unrelated action must not be blocked behind the transcription concurrency cap")

	// Exactly one transcription ever ran its backend at a time.
	assert.Equal(t, int32(1), tracker.max.Load(), "transcriptions must run sequentially, not concurrently")
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
