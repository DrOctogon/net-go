package speaker

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
// When the feature is disabled, or no model path is configured, it returns a
// NoopAnalyzer. When cfg.Enabled is set AND at least one model path is provided,
// it loads the real ONNX-backed analyzer from the first non-empty of
// GenderModelPath/AgeModelPath/VoicePrintModelPath. A single audEERING wav2vec2
// model file serves all three outputs (age, gender, voice-print), so only one
// path is needed; the per-attribute sub-flags decide which outputs are used.
// A construction failure (bad path, unroutable model I/O) is returned to the
// caller, which logs it and falls back to NoopAnalyzer.
//
// PRIVACY CONTRACT: the returned analyzer MUST populate ONLY the Attributes
// fields whose sub-feature is enabled — GenderEnabled gates Gender/
// GenderConfidence, AgeEnabled gates AgeBand/AgeConfidence, VoicePrintEnabled
// gates Embedding. A user who turns the master switch on but a sub-attribute off
// has opted OUT of that inference being emitted; surfacing it anyway is a privacy
// regression. This gating now happens inside ONNXAnalyzer (see onnx_analyzer.go);
// TestNewHonorsSubFeatureFlags locks the contract in.
func New(cfg Config) (Analyzer, error) {
	if !cfg.Enabled {
		return NoopAnalyzer{}, nil
	}

	modelPath := firstNonEmpty(cfg.GenderModelPath, cfg.AgeModelPath, cfg.VoicePrintModelPath)
	if modelPath == "" {
		return NoopAnalyzer{}, nil
	}

	analyzer, err := newONNXAnalyzer(modelPath, cfg)
	if err != nil {
		return nil, err
	}
	return analyzer, nil
}

// firstNonEmpty returns the first non-empty string in paths, or "" if all are
// empty.
func firstNonEmpty(paths ...string) string {
	for _, p := range paths {
		if p != "" {
			return p
		}
	}
	return ""
}
