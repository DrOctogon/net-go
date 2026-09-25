// actions_speaker_test.go - Tests for analyzeSpeakerAttributes and
// emitSpeakerAttributeAlert (see actions_speaker.go).
//
// These tests verify the speaker-attribute analysis seam WITHOUT requiring a
// real ONNX model: a fake speaker.Analyzer stands in for the bundled
// NoopAnalyzer / ONNX-backed implementation. Coverage focuses on control flow:
//   - no-op when the feature is disabled (hot-reload safe)
//   - no-op when no analyzer is configured
//   - no-op when there is no audio to analyze
//   - analyzer errors never populate attributes and never panic
//   - a successful analysis attaches attributes onto detection.Result
//   - a nil ctx falls back to context.Background() instead of panicking
//   - the speaker clusterer assigns a SpeakerID only when it is present and a
//     non-empty embedding was produced
package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/detection"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// fakeSpeakerAnalyzer is a test double for speaker.Analyzer. It records how
// many times Analyze was called and the ctx it was called with, so tests can
// assert the seam's short-circuit and nil-ctx-guard behavior without a real
// ONNX model.
type fakeSpeakerAnalyzer struct {
	attrs   speaker.Attributes
	err     error
	calls   int
	lastCtx context.Context //nolint:containedctx // captured for test assertions only
}

func (f *fakeSpeakerAnalyzer) Analyze(ctx context.Context, _ [][]float32) (speaker.Attributes, error) {
	f.calls++
	f.lastCtx = ctx
	return f.attrs, f.err
}

// speakerAttrSettings returns settings with the speaker-attributes master
// switch set as requested; all model sub-settings are left at their zero
// value since analyzeSpeakerAttributes only consults the master switch.
func speakerAttrSettings(enabled bool) *conf.Settings {
	s := &conf.Settings{}
	s.Realtime.Audio.SpeakerAttributes.Enabled = enabled
	return s
}

// pendingDetectionWithPCM returns a *PendingDetection carrying non-empty
// 3s PCM data, so analyzeSpeakerAttributes proceeds past the empty-audio
// guard.
func pendingDetectionWithPCM() *PendingDetection {
	return &PendingDetection{
		Detection: Detections{
			CorrelationID: "test-correlation-id",
			pcmData3s:     make([]byte, 32), // 16 s16le samples; content irrelevant to the fake analyzer
		},
	}
}

// withFallbackSettings ensures conf.CurrentOrFallback (used by
// Processor.currentSettings) falls back to the Processor's injected settings
// rather than a global snapshot possibly left behind by another test in this
// package. Restored on test cleanup.
func withFallbackSettings(t *testing.T) {
	t.Helper()
	conf.StoreSettings(nil)
	t.Cleanup(func() { conf.StoreSettings(nil) })
}

func TestAnalyzeSpeakerAttributes_ShortCircuits(t *testing.T) {
	tests := []struct {
		name     string
		settings *conf.Settings
		analyzer speaker.Analyzer
		item     *PendingDetection
	}{
		{
			name:     "disabled settings",
			settings: speakerAttrSettings(false),
			analyzer: &fakeSpeakerAnalyzer{},
			item:     pendingDetectionWithPCM(),
		},
		{
			name:     "nil settings",
			settings: nil,
			analyzer: &fakeSpeakerAnalyzer{},
			item:     pendingDetectionWithPCM(),
		},
		{
			name:     "nil analyzer",
			settings: speakerAttrSettings(true),
			analyzer: nil,
			item:     pendingDetectionWithPCM(),
		},
		{
			name:     "no pcm data",
			settings: speakerAttrSettings(true),
			analyzer: &fakeSpeakerAnalyzer{},
			item:     &PendingDetection{Detection: Detections{pcmData3s: nil}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFallbackSettings(t)

			p := &Processor{Settings: tt.settings}
			p.speakerAnalyzer = tt.analyzer

			assert.NotPanics(t, func() {
				p.analyzeSpeakerAttributes(t.Context(), tt.item)
			})

			if fake, ok := tt.analyzer.(*fakeSpeakerAnalyzer); ok {
				assert.Zero(t, fake.calls, "analyzer must not be invoked when short-circuited")
			}
			assert.Empty(t, tt.item.Detection.Result.Gender, "result must be left untouched")
			assert.Empty(t, tt.item.Detection.Result.AgeBand, "result must be left untouched")
		})
	}
}

func TestAnalyzeSpeakerAttributes_AnalyzerError_LeavesResultEmpty(t *testing.T) {
	withFallbackSettings(t)

	fake := &fakeSpeakerAnalyzer{err: errors.New("model exploded")}
	p := &Processor{Settings: speakerAttrSettings(true)}
	p.speakerAnalyzer = fake

	item := pendingDetectionWithPCM()

	assert.NotPanics(t, func() {
		p.analyzeSpeakerAttributes(t.Context(), item)
	})

	assert.Equal(t, 1, fake.calls, "analyzer should have been invoked once")
	r := item.Detection.Result
	assert.Empty(t, r.Gender)
	assert.Empty(t, r.AgeBand)
	assert.Empty(t, r.VoicePrintEmbedding)
	assert.Empty(t, r.SpeakerID)
}

func TestAnalyzeSpeakerAttributes_Success_PopulatesResult(t *testing.T) {
	withFallbackSettings(t)

	fake := &fakeSpeakerAnalyzer{attrs: speaker.Attributes{
		Gender:           speaker.GenderFemale,
		GenderConfidence: 0.91,
		AgeBand:          speaker.AgeBandAdult,
		AgeConfidence:    0.77,
		Embedding:        []float32{0.1, 0.2, 0.3},
	}}
	p := &Processor{Settings: speakerAttrSettings(true)}
	p.speakerAnalyzer = fake
	// No clusterer configured: SpeakerID must stay unset regardless of embedding.
	require.Nil(t, p.speakerClusterer)

	item := pendingDetectionWithPCM()
	p.analyzeSpeakerAttributes(t.Context(), item)

	require.Equal(t, 1, fake.calls)
	r := item.Detection.Result
	assert.Equal(t, speaker.GenderFemale, r.Gender)
	assert.InDelta(t, 0.91, r.GenderConfidence, 1e-9)
	assert.Equal(t, speaker.AgeBandAdult, r.AgeBand)
	assert.InDelta(t, 0.77, r.AgeConfidence, 1e-9)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, r.VoicePrintEmbedding)
	assert.Empty(t, r.SpeakerID, "no clusterer configured -> no SpeakerID assigned")
}

func TestAnalyzeSpeakerAttributes_NilCtx_FallsBackToBackground(t *testing.T) {
	withFallbackSettings(t)

	fake := &fakeSpeakerAnalyzer{attrs: speaker.Attributes{Gender: speaker.GenderMale}}
	p := &Processor{Settings: speakerAttrSettings(true)}
	p.speakerAnalyzer = fake

	item := pendingDetectionWithPCM()

	assert.NotPanics(t, func() {
		//nolint:staticcheck // deliberately passing nil to exercise the ctx==nil guard
		p.analyzeSpeakerAttributes(nil, item)
	})

	require.Equal(t, 1, fake.calls)
	require.NotNil(t, fake.lastCtx, "nil ctx must be replaced with a non-nil context")
	assert.Equal(t, speaker.GenderMale, item.Detection.Result.Gender)
}

func TestAnalyzeSpeakerAttributes_Clusterer_AssignsSpeakerIDOnlyWithEmbedding(t *testing.T) {
	tests := []struct {
		name          string
		clusterer     *speaker.Clusterer
		embedding     []float32
		wantSpeakerID string
	}{
		{
			name:          "clusterer present, non-empty embedding -> assigned",
			clusterer:     speaker.NewClusterer(0),
			embedding:     []float32{1, 0, 0, 0},
			wantSpeakerID: "spk_1",
		},
		{
			name:          "clusterer present, empty embedding -> not assigned",
			clusterer:     speaker.NewClusterer(0),
			embedding:     nil,
			wantSpeakerID: "",
		},
		{
			name:          "clusterer nil -> never assigned even with embedding",
			clusterer:     nil,
			embedding:     []float32{1, 0, 0, 0},
			wantSpeakerID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFallbackSettings(t)

			fake := &fakeSpeakerAnalyzer{attrs: speaker.Attributes{
				Gender:    speaker.GenderUnknown,
				Embedding: tt.embedding,
			}}
			p := &Processor{Settings: speakerAttrSettings(true)}
			p.speakerAnalyzer = fake
			p.speakerClusterer = tt.clusterer

			item := pendingDetectionWithPCM()
			p.analyzeSpeakerAttributes(t.Context(), item)

			assert.Equal(t, tt.wantSpeakerID, item.Detection.Result.SpeakerID)
		})
	}
}

func TestEmitSpeakerAttributeAlert_ShortCircuits(t *testing.T) {
	attrs := &detection.Result{Gender: speaker.GenderFemale, GenderConfidence: 0.5}

	tests := []struct {
		name     string
		settings *conf.Settings
		result   *detection.Result
	}{
		{name: "nil settings", settings: nil, result: attrs},
		{name: "disabled settings", settings: speakerAttrSettings(false), result: attrs},
		{name: "nil result", settings: speakerAttrSettings(true), result: nil},
		{name: "result without attributes", settings: speakerAttrSettings(true), result: &detection.Result{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				emitSpeakerAttributeAlert(tt.settings, tt.result)
			})
		})
	}
}

func TestEmitSpeakerAttributeAlert_WithAttributes_NoPanic(t *testing.T) {
	// No global alert bus is configured in this package's tests by default, so
	// TryPublish is a safe no-op; this exercises the confidence-max branch and
	// the publish call without needing a live subscriber.
	r := &detection.Result{
		ID:               1,
		Gender:           speaker.GenderMale,
		GenderConfidence: 0.4,
		AgeBand:          speaker.AgeBandTeen,
		AgeConfidence:    0.8,
	}

	assert.NotPanics(t, func() {
		emitSpeakerAttributeAlert(speakerAttrSettings(true), r)
	})
}
