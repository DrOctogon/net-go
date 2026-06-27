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
// No real gender/age/voice-print ONNX model is vendored yet, so this currently
// always returns a NoopAnalyzer. When models are added, this is the single
// place to construct the real ONNX-backed analyzer (mirroring the Silero VAD
// wrapper in internal/classifier/humanvoice). The seam, persistence columns,
// search filters, and alert rule are already wired, so only this constructor
// and the analyzer implementation need to change.
//
// PRIVACY CONTRACT (enforce when wiring the real model): the returned analyzer
// MUST honor the per-attribute sub-flags and populate ONLY the Attributes fields
// whose sub-feature is enabled — GenderEnabled gates Gender/GenderConfidence,
// AgeEnabled gates AgeBand/AgeConfidence, VoicePrintEnabled gates Embedding.
// A user who turns the master switch on but a sub-attribute off has opted OUT of
// generating that inference; running the model for it anyway is a privacy
// regression. TestNewHonorsSubFeatureFlags locks this contract in.
func New(cfg Config) Analyzer {
	return NoopAnalyzer{}
}
