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
func New(cfg Config) Analyzer {
	return NoopAnalyzer{}
}
