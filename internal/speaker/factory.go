package speaker

import "github.com/tphakala/voicewatch/internal/inference/onnx"

// speakerModelThreads is the per-model ONNX intra/inter-op thread count.
// 0 => use the ONNX session default thread count (see createSession).
const speakerModelThreads = 0

// Config controls which speaker analyses run and where the models load from.
// It is a transport-agnostic mirror of conf.SpeakerAttributesSettings so this
// package does not depend on the configuration package.
type Config struct {
	Enabled             bool
	GenderEnabled       bool
	AgeEnabled          bool
	VoicePrintEnabled   bool
	GenderModelPath     string
	AgeModelPath        string
	VoicePrintModelPath string
}

// New returns an Analyzer for the given config.
//
// Each attribute is served by its own INDEPENDENT single-output ONNX model,
// composed à la carte:
//   - gender:      cfg.GenderModelPath      when cfg.GenderEnabled
//   - age:         cfg.AgeModelPath         when cfg.AgeEnabled
//   - voice-print: cfg.VoicePrintModelPath  when cfg.VoicePrintEnabled
//
// A model is built only when its sub-feature is enabled AND its path is set.
// When cfg.Enabled is false, or no model ends up being built, New returns a
// NoopAnalyzer. If any model fails to construct (bad path, unroutable model
// I/O), New closes any already-built models and returns the error; the caller
// logs it and falls back to NoopAnalyzer.
//
// PRIVACY CONTRACT: a disabled attribute's model is never built, so it can never
// be run and never emitted — there is no shared inference that could leak a
// disabled attribute. GenderEnabled gates Gender/GenderConfidence, AgeEnabled
// gates AgeBand/AgeConfidence, VoicePrintEnabled gates Embedding.
// TestNewHonorsSubFeatureFlags locks the contract in.
func New(cfg Config) (Analyzer, error) {
	if !cfg.Enabled {
		return NoopAnalyzer{}, nil
	}

	b := &modelBuilder{threads: speakerModelThreads}
	genderModel := b.build(cfg.GenderEnabled, cfg.GenderModelPath)
	ageModel := b.build(cfg.AgeEnabled, cfg.AgeModelPath)
	voicePrintModel := b.build(cfg.VoicePrintEnabled, cfg.VoicePrintModelPath)
	if b.err != nil {
		b.closeAll()
		return nil, b.err
	}

	if genderModel == nil && ageModel == nil && voicePrintModel == nil {
		return NoopAnalyzer{}, nil
	}
	return newONNXAnalyzer(genderModel, ageModel, voicePrintModel), nil
}

// modelBuilder constructs per-attribute models, accumulating the first error and
// tracking successfully-built models so they can be released on failure.
type modelBuilder struct {
	threads int
	built   []*onnx.SingleOutputAudioModel
	err     error
}

// build loads a model from path when enabled and path is non-empty. It returns
// nil (without setting err) when the attribute is disabled or unconfigured, and
// nil (setting err) on a construction failure. Once err is set, subsequent calls
// short-circuit to nil so the first error is preserved.
func (b *modelBuilder) build(enabled bool, path string) *onnx.SingleOutputAudioModel {
	if b.err != nil || !enabled || path == "" {
		return nil
	}
	// normalize=false: pass the raw waveform through. Matches models with an
	// in-graph front end (e.g. the ECAPA gender model, which does its own
	// preemphasis + mel + mean-norm). A Wav2Vec2-style model that needs external
	// waveform normalization would require a per-attribute normalize config flag
	// (not yet plumbed through conf.SpeakerAttributesSettings).
	m, err := onnx.NewSingleOutputAudioModel(path, b.threads, false)
	if err != nil {
		b.err = err
		return nil
	}
	b.built = append(b.built, m)
	return m
}

// closeAll releases every model built so far. Used to avoid leaking sessions
// when a later model fails to construct.
func (b *modelBuilder) closeAll() {
	for _, m := range b.built {
		_ = m.Close()
	}
}
