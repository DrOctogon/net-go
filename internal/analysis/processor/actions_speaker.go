// processor/actions_speaker.go
// Speaker-attribute estimation (gender, relative age band) and voice-print
// embedding for approved human-voice detections. This runs at the detection
// approval seam, before actions are built, so the estimates are persisted with
// the detection via NoteFromResult. It is a no-op (and returns immediately)
// unless the opt-in speaker-attributes feature is enabled.

package processor

import (
	"context"

	"github.com/tphakala/voicewatch/internal/alerting"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/detection"
	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// analyzeSpeakerAttributes estimates speaker attributes for an approved
// detection and attaches them to the detection Result so they persist on save.
// It is a no-op when the feature is disabled, the analyzer is unset, or no
// audio is available. Errors never abort the detection — they are logged and
// the detection saves without attributes.
//
// The ctx is the processor's flusher lifecycle context, so a real (blocking)
// ONNX model honors shutdown cancellation. The bundled NoopAnalyzer ignores it.
//
// Estimation happens here (pre-save) so the attributes ride into the database
// via NoteFromResult. The matching alert is emitted post-save by
// emitSpeakerAttributeAlert (from DatabaseAction) so it carries a persisted
// detection ID — at this seam the ID is not yet assigned.
func (p *Processor) analyzeSpeakerAttributes(ctx context.Context, item *PendingDetection) {
	settings := p.currentSettings()
	if settings == nil || !settings.Realtime.Audio.SpeakerAttributes.Enabled {
		return
	}
	analyzer := p.speakerAnalyzer
	if analyzer == nil {
		return
	}

	pcm := item.Detection.pcmData3s
	if len(pcm) == 0 {
		return
	}

	// The clip is carried as 16 kHz mono s16le bytes; convert to the single
	// mono channel the analyzer expects.
	samples := speaker.PCMS16LEToFloat32(pcm)
	if len(samples) == 0 {
		return
	}

	// flusherCtx may be unset on code paths that bypass processor start (e.g.
	// direct unit tests); fall back to a non-nil context for the model call.
	if ctx == nil {
		ctx = context.Background()
	}
	attrs, err := analyzer.Analyze(ctx, [][]float32{samples})
	if err != nil {
		GetLogger().Warn("Speaker-attribute analysis failed",
			logger.String("component", "analysis.processor.actions"),
			logger.String("detection_id", item.Detection.CorrelationID),
			logger.Error(err),
			logger.String("operation", "speaker_attributes"))
		return
	}

	// Attach to the domain Result; NoteFromResult maps these onto the saved row.
	r := &item.Detection.Result
	r.Gender = attrs.Gender
	r.GenderConfidence = attrs.GenderConfidence
	r.AgeBand = attrs.AgeBand
	r.AgeConfidence = attrs.AgeConfidence
	r.SpeakerID = attrs.SpeakerID
	r.VoicePrintEmbedding = attrs.Embedding
}

// emitSpeakerAttributeAlert publishes a speaker.attribute_matched alert for a
// detection that has been persisted, so notification rules can fire on estimated
// attributes with a valid detection ID. It is called post-save from
// DatabaseAction. No-op when the feature is disabled or the detection carries no
// estimated attributes (the common case, including the bundled NoopAnalyzer).
func emitSpeakerAttributeAlert(settings *conf.Settings, r *detection.Result) {
	if settings == nil || !settings.Realtime.Audio.SpeakerAttributes.Enabled {
		return
	}
	if r == nil || !r.HasSpeakerAttributes() {
		return
	}

	// confidence is the max of the two independent model confidences — a
	// conservative upper bound for rule thresholds, NOT a joint posterior.
	confidence := r.GenderConfidence
	if r.AgeConfidence > confidence {
		confidence = r.AgeConfidence
	}
	alerting.TryPublish(&alerting.AlertEvent{
		ObjectType: alerting.ObjectTypeSpeakerAttr,
		EventName:  alerting.EventSpeakerAttributeMatched,
		Properties: map[string]any{
			alerting.PropertySpeakerGender:  r.Gender,
			alerting.PropertySpeakerAgeBand: r.AgeBand,
			alerting.PropertyConfidence:     confidence,
			alerting.PropertyDetectionID:    r.ID,
		},
	})
}
