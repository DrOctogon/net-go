package speaker

import (
	"context"

	"github.com/tphakala/voicewatch/internal/inference/onnx"
)

// ONNXAnalyzer is an Analyzer backed by up to three independent per-attribute
// ONNX models. Each model is optional (any field may be nil); a model is built
// only when its sub-feature is enabled AND its model path is configured (see
// New in factory.go).
//
// PRIVACY CONTRACT (structural): a disabled attribute's model is never built, so
// it is nil here, so it is never run, so the attribute is never emitted. There is
// no shared inference to leak across attributes the way the old combined model
// could. TestNewHonorsSubFeatureFlags locks this in.
//
// NOT goroutine-safe: each underlying model serializes inference, so callers must
// not invoke Analyze concurrently.
type ONNXAnalyzer struct {
	genderModel     *onnx.SingleOutputAudioModel
	ageModel        *onnx.SingleOutputAudioModel
	voicePrintModel *onnx.SingleOutputAudioModel
}

// newONNXAnalyzer wraps the already-constructed per-attribute models. At least
// one model is expected to be non-nil (the factory returns a NoopAnalyzer when
// none are built).
func newONNXAnalyzer(genderModel, ageModel, voicePrintModel *onnx.SingleOutputAudioModel) *ONNXAnalyzer {
	return &ONNXAnalyzer{
		genderModel:     genderModel,
		ageModel:        ageModel,
		voicePrintModel: voicePrintModel,
	}
}

// Analyze implements Analyzer. It runs each non-nil model on the mono channel
// (samples[0]) and maps the output to the corresponding attribute. Empty input
// returns empty Attributes and no error. Context cancellation is honored before
// and after inference.
//
// MULTI-MODEL ERROR SEMANTICS: each model is run independently. A per-model
// inference failure leaves that attribute unpopulated and does NOT abort the
// others (log-and-continue per attribute). An error is returned only if every
// model that was attempted failed (the first such error); otherwise Analyze
// returns whatever attributes succeeded with a nil error. This keeps a single
// flaky attribute from suppressing the others while still surfacing a total
// failure to the caller (which logs and continues with empty Attributes).
func (a *ONNXAnalyzer) Analyze(ctx context.Context, samples [][]float32) (Attributes, error) {
	if err := ctx.Err(); err != nil {
		return Attributes{}, err
	}
	if len(samples) == 0 || len(samples[0]) == 0 {
		return Attributes{}, nil
	}
	mono := samples[0]

	var attrs Attributes
	var firstErr error
	attempted := 0
	failed := 0

	recordErr := func(err error) {
		failed++
		if firstErr == nil {
			firstErr = err
		}
	}

	if a.genderModel != nil {
		attempted++
		out, err := a.genderModel.Infer(mono)
		switch {
		case err != nil:
			recordErr(err)
		case len(out) > 0:
			attrs.Gender, attrs.GenderConfidence = mapGender(out)
		}
	}

	if a.ageModel != nil {
		attempted++
		out, err := a.ageModel.Infer(mono)
		switch {
		case err != nil:
			recordErr(err)
		case len(out) > 0:
			attrs.AgeBand, attrs.AgeConfidence = mapAge(out[0])
		}
	}

	if a.voicePrintModel != nil {
		attempted++
		out, err := a.voicePrintModel.Infer(mono)
		switch {
		case err != nil:
			recordErr(err)
		case len(out) > 0:
			// Infer returns a caller-owned slice, so the analyzer may retain it.
			attrs.Embedding = out
		}
	}

	if err := ctx.Err(); err != nil {
		return Attributes{}, err
	}

	// Surface an error only when every attempted model failed.
	if attempted > 0 && failed == attempted {
		return Attributes{}, firstErr
	}
	return attrs, nil
}

// Close releases all non-nil underlying ONNX models, returning the first close
// error encountered (if any).
func (a *ONNXAnalyzer) Close() error {
	var firstErr error
	for _, m := range []*onnx.SingleOutputAudioModel{a.genderModel, a.ageModel, a.voicePrintModel} {
		if m == nil {
			continue
		}
		if err := m.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
