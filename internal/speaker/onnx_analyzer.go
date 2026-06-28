package speaker

import (
	"context"

	"github.com/tphakala/voicewatch/internal/inference/onnx"
)

// ONNXAnalyzer is an Analyzer backed by the audEERING wav2vec2 age/gender/
// voice-print ONNX model. A single combined model file produces all three
// outputs; the per-attribute sub-flags decide which of them are populated,
// enforcing the privacy contract documented on New (a disabled sub-feature is
// never emitted even though the same inference produced it).
//
// NOT goroutine-safe: the underlying model serializes inference, so callers must
// not invoke Analyze concurrently.
type ONNXAnalyzer struct {
	model             *onnx.AgeGenderModel
	genderEnabled     bool
	ageEnabled        bool
	voicePrintEnabled bool
}

// newONNXAnalyzer loads the combined age/gender/voice-print model from modelPath
// and wires the per-attribute gating from cfg. It returns an error if the model
// cannot be loaded (e.g. bad path or unroutable I/O).
func newONNXAnalyzer(modelPath string, cfg Config) (*ONNXAnalyzer, error) {
	model, err := onnx.NewAgeGenderModel(modelPath, 0)
	if err != nil {
		return nil, err
	}
	return &ONNXAnalyzer{
		model:             model,
		genderEnabled:     cfg.GenderEnabled,
		ageEnabled:        cfg.AgeEnabled,
		voicePrintEnabled: cfg.VoicePrintEnabled,
	}, nil
}

// Analyze implements Analyzer. It runs the model on the mono channel
// (samples[0]) and maps the outputs to Attributes, populating only the
// sub-features that are enabled. Empty input returns empty Attributes and no
// error. Context cancellation is honored before and after inference.
func (a *ONNXAnalyzer) Analyze(ctx context.Context, samples [][]float32) (Attributes, error) {
	if err := ctx.Err(); err != nil {
		return Attributes{}, err
	}
	if len(samples) == 0 || len(samples[0]) == 0 {
		return Attributes{}, nil
	}
	mono := samples[0]

	ageScore, genderLogits, embedding, err := a.model.Infer(mono)
	if err != nil {
		return Attributes{}, err
	}
	if err := ctx.Err(); err != nil {
		return Attributes{}, err
	}

	var attrs Attributes
	if a.genderEnabled {
		attrs.Gender, attrs.GenderConfidence = mapGender(genderLogits)
	}
	if a.ageEnabled {
		attrs.AgeBand, attrs.AgeConfidence = mapAge(ageScore)
	}
	if a.voicePrintEnabled && len(embedding) > 0 {
		// Infer returns a caller-owned slice, so the analyzer may retain it.
		attrs.Embedding = embedding
	}
	return attrs, nil
}

// Close releases the underlying ONNX model.
func (a *ONNXAnalyzer) Close() error {
	return a.model.Close()
}
