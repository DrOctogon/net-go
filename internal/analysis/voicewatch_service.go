package analysis

import (
	"context"
	"path/filepath"

	"github.com/tphakala/voicewatch/internal/app"
	"github.com/tphakala/voicewatch/internal/classifier"
	"github.com/tphakala/voicewatch/internal/classifier/humanvoice"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/events"
	"github.com/tphakala/voicewatch/internal/logger"
)

// voiceWatchAnalyzerName is the service name used for logging and diagnostics.
const voiceWatchAnalyzerName = "voicewatch-analyzer"

// VoiceWatchAnalyzer wraps the VoiceWatch model initialization as an app.Service
// and implements app.Analyzer for source-to-analyzer routing.
type VoiceWatchAnalyzer struct {
	settings *conf.Settings
	bn       *classifier.Orchestrator
}

// NewVoiceWatchAnalyzer creates a new VoiceWatchAnalyzer with the given settings.
// The analyzer is not started; call Start() to initialize the VoiceWatch model.
func NewVoiceWatchAnalyzer(settings *conf.Settings) *VoiceWatchAnalyzer {
	return &VoiceWatchAnalyzer{settings: settings}
}

// Name returns a human-readable identifier for logging and diagnostics.
func (a *VoiceWatchAnalyzer) Name() string {
	return voiceWatchAnalyzerName
}

// Start initializes the VoiceWatch speech model.
// Model initialization failures are non-retryable (missing files, insufficient resources).
func (a *VoiceWatchAnalyzer) Start(_ context.Context) error {
	// Resolve the Silero VAD model path. When the operator has not configured an
	// explicit path, extract the model embedded in the binary into the models
	// directory and use that.
	modelPath := a.settings.VoiceWatch.ModelPath
	if modelPath == "" {
		configDir, dirErr := conf.ResolveConfigDir()
		if dirErr != nil {
			return errors.New(dirErr).
				Component("analysis").
				Category(errors.CategoryModelInit).
				Context("operation", "resolve_models_dir").
				Build()
		}
		modelsDir := filepath.Join(configDir, "models")
		extracted, extractErr := humanvoice.WriteEmbeddedModel(modelsDir)
		if extractErr != nil {
			return errors.New(extractErr).
				Component("analysis").
				Category(errors.CategoryModelInit).
				Context("operation", "extract_embedded_humanvoice_model").
				Build()
		}
		modelPath = extracted
	}

	// Construct the single human-voice (Silero VAD) model in the analysis layer and
	// inject it into the orchestrator. Building the model here (rather than inside
	// classifier) avoids a humanvoice -> classifier import cycle.
	model, err := humanvoice.New(&humanvoice.Config{
		ModelPath:       modelPath,
		ONNXRuntimePath: a.settings.VoiceWatch.ONNXRuntimePath,
		Threads:         a.settings.VoiceWatch.Threads,
		Threshold:       a.settings.VoiceWatch.Threshold,
	})
	if err != nil {
		return errors.New(err).
			Component("analysis").
			Category(errors.CategoryModelInit).
			Context("operation", "initialize_humanvoice").
			Build()
	}

	bn, err := classifier.NewOrchestrator(a.settings, model)
	if err != nil {
		_ = model.Close()
		return errors.New(err).
			Component("analysis").
			Category(errors.CategoryModelInit).
			Context("operation", "initialize_orchestrator").
			Build()
	}

	a.bn = bn

	events.Emit(context.Background(), "detection", "model_loaded", "Human-voice model loaded", map[string]any{
		"species_count": bn.NumSpecies(),
	})

	return nil
}

// Stop releases VoiceWatch model resources. It is safe to call before Start()
// or multiple times.
func (a *VoiceWatchAnalyzer) Stop(_ context.Context) error {
	if a.bn != nil {
		log := GetLogger()
		log.Info("stopping VoiceWatch model",
			logger.String("service", voiceWatchAnalyzerName))
		a.bn.Delete()
		a.bn = nil
	}
	return nil
}

// Compatible returns true if this analyzer can process audio from the given source.
// VoiceWatchAnalyzer handles all source types except ultrasonic (bat detection).
func (a *VoiceWatchAnalyzer) Compatible(source app.AudioSource) bool {
	return source.Type != app.SourceTypeUltrasonic
}

// Orchestrator returns the underlying classifier orchestrator, or nil if the analyzer
// has not been started. Callers must not use the returned pointer after Stop().
func (a *VoiceWatchAnalyzer) Orchestrator() *classifier.Orchestrator {
	return a.bn
}

