// processor/actions_voice.go
// This file contains the speech-to-text transcription action. It mirrors the
// SSEAction/MqttAction shape: a mutex-guarded action that early-returns when the
// feature is disabled or the backend is unavailable, persists its result via the
// detection repository, and shares the result through the DetectionContext.

package processor

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/tphakala/voicewatch/internal/analysis/jobqueue"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/detection"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/transcription"
)

// Transcription retry tuning. Transcription runs as an independent job after the
// Database -> SSE -> MQTT pipeline, so the detection ID (set by DatabaseAction)
// and the exported clip (written by SaveAudioAction) may not be ready when the
// job first runs. These retries let the job back off and pick up the work once
// both prerequisites are available, without ever blocking SSE/MQTT.
const (
	transcribeMaxRetries   = 12
	transcribeInitialDelay = 2 * time.Second
	transcribeMaxDelay     = 10 * time.Second
	transcribeMultiplier   = 1.5
)

// TranscribeAction transcribes a saved audio clip to text using the configured
// speech-to-text backend and persists the transcript onto the detection. It runs
// off the hot path as its own job so slow transcription never blocks the
// Database/SSE/MQTT pipeline.
type TranscribeAction struct {
	Settings      *conf.Settings
	Result        detection.Result              // Domain model (single source of truth)
	Transcriber   transcription.Transcriber     // Backend (whisper.cpp CLI); may be unavailable
	Repo          datastore.DetectionRepository // Preferred path for persistence
	EventTracker  *EventTracker
	DetectionCtx  *DetectionContext    // Shared context from DatabaseAction (provides detection ID)
	RetryConfig   jobqueue.RetryConfig // Retry behavior while prerequisites become available
	ClipName      string               // Saved clip file name, resolved against the export path
	Description   string
	CorrelationID string     // Detection correlation ID for log tracking
	mu            sync.Mutex // Protect concurrent access to Result
}

// GetDescription returns a human-readable description of the TranscribeAction.
func (a *TranscribeAction) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return "Transcribe detection audio clip to text"
}

// Execute transcribes the saved clip and stores the transcript. It is a no-op
// when transcription is disabled (hot-reload safe) or the backend is unavailable.
// When the detection ID or the clip file is not yet ready it returns a retryable
// error so the job queue retries until both prerequisites exist.
func (a *TranscribeAction) Execute(ctx context.Context, _ any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Hot-reload: respect the current enabled state even if this action was
	// queued before the setting was toggled off.
	if a.Settings == nil || !a.Settings.Realtime.Transcription.Enabled {
		return nil
	}

	// Skip cleanly when no backend is configured or the binary/model is missing.
	// Available() is the graceful-skip gate, not an error condition.
	if a.Transcriber == nil || !a.Transcriber.Available() {
		return nil
	}

	// The detection ID is assigned by DatabaseAction and shared via DetectionCtx.
	// Without it the transcript cannot be persisted; retry until it is set.
	var noteID uint
	if a.DetectionCtx != nil {
		noteID = uint(a.DetectionCtx.NoteID.Load())
	}
	if noteID == 0 {
		return errors.Newf("transcription deferred: detection ID not yet assigned").
			Component("analysis.processor").
			Category(errors.CategoryProcessing).
			Context("operation", "transcribe").
			Context("retryable", true).
			Build()
	}

	// Resolve the saved clip path against the configured export directory.
	clipName := a.ClipName
	if clipName == "" {
		clipName = a.Result.ClipName
	}
	if clipName == "" {
		// No clip was requested for this detection; nothing to transcribe.
		return nil
	}
	clipPath := filepath.Join(a.Settings.Realtime.Audio.Export.Path, clipName)

	result, err := a.Transcriber.Transcribe(ctx, clipPath)
	if err != nil {
		// The clip may still be encoding (SaveAudioAction runs independently), so
		// treat transcription failures as retryable.
		GetLogger().Warn("Transcription failed",
			logger.String("component", "analysis.processor.actions"),
			logger.String("detection_id", a.CorrelationID),
			logger.String("clip_name", clipName),
			logger.Error(err),
			logger.String("operation", "transcribe"))
		return errors.New(err).
			Component("analysis.processor").
			Category(errors.CategoryProcessing).
			Context("operation", "transcribe").
			Context("clip_name", clipName).
			Context("retryable", true).
			Build()
	}

	// Share the transcript with any late consumers via the detection context.
	if a.DetectionCtx != nil {
		a.DetectionCtx.SetTranscript(result.Text, result.Language)
	}

	// Persist the transcript onto the detection.
	if a.Repo != nil {
		id := strconv.FormatUint(uint64(noteID), 10)
		if err := a.Repo.UpdateTranscript(ctx, id, result.Text, result.Language); err != nil {
			GetLogger().Error("Failed to persist transcript",
				logger.String("component", "analysis.processor.actions"),
				logger.String("detection_id", a.CorrelationID),
				logger.Uint64("note_id", uint64(noteID)),
				logger.Error(err),
				logger.String("operation", "transcribe_persist"))
			return errors.New(err).
				Component("analysis.processor").
				Category(errors.CategoryDatabase).
				Context("operation", "transcribe_persist").
				Context("note_id", noteID).
				Context("retryable", true).
				Build()
		}
	}

	if a.Settings.Debug {
		GetLogger().Debug("Transcribed detection audio clip",
			logger.String("component", "analysis.processor.actions"),
			logger.String("detection_id", a.CorrelationID),
			logger.Uint64("note_id", uint64(noteID)),
			logger.String("language", result.Language),
			logger.Int64("duration_ms", result.DurationMs),
			logger.String("operation", "transcribe_success"))
	}

	return nil
}

// newTranscribeRetryConfig returns the retry configuration used while the
// transcription prerequisites (detection ID and exported clip) become available.
func newTranscribeRetryConfig() jobqueue.RetryConfig {
	return jobqueue.RetryConfig{
		Enabled:      true,
		MaxRetries:   transcribeMaxRetries,
		InitialDelay: transcribeInitialDelay,
		MaxDelay:     transcribeMaxDelay,
		Multiplier:   transcribeMultiplier,
	}
}
