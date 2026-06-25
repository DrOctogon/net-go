package analysis

import (
	"context"

	"github.com/tphakala/birdnet-go/internal/app"
	"github.com/tphakala/birdnet-go/internal/classifier"
	"github.com/tphakala/birdnet-go/internal/classifier/humanvoice"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/events"
	"github.com/tphakala/birdnet-go/internal/logger"
)

// birdNETAnalyzerName is the service name used for logging and diagnostics.
const birdNETAnalyzerName = "birdnet-analyzer"

// BirdNETAnalyzer wraps BirdNET model initialization as an app.Service
// and implements app.Analyzer for source-to-analyzer routing.
type BirdNETAnalyzer struct {
	settings *conf.Settings
	bn       *classifier.Orchestrator
}

// NewBirdNETAnalyzer creates a new BirdNETAnalyzer with the given settings.
// The analyzer is not started; call Start() to initialize the BirdNET model.
func NewBirdNETAnalyzer(settings *conf.Settings) *BirdNETAnalyzer {
	return &BirdNETAnalyzer{settings: settings}
}

// Name returns a human-readable identifier for logging and diagnostics.
func (a *BirdNETAnalyzer) Name() string {
	return birdNETAnalyzerName
}

// Start initializes the BirdNET interpreter and builds the species range filter.
// Model initialization failures are non-retryable (missing files, insufficient resources).
func (a *BirdNETAnalyzer) Start(_ context.Context) error {
	// Construct the single human-voice (Silero VAD) model in the analysis layer and
	// inject it into the orchestrator. Building the model here (rather than inside
	// classifier) avoids a humanvoice -> classifier import cycle.
	model, err := humanvoice.New(&humanvoice.Config{
		ModelPath:       a.settings.BirdNET.ModelPath,
		ONNXRuntimePath: a.settings.BirdNET.ONNXRuntimePath,
		Threads:         a.settings.BirdNET.Threads,
		Threshold:       a.settings.BirdNET.Threshold,
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

// Stop releases BirdNET model resources. It is safe to call before Start()
// or multiple times.
func (a *BirdNETAnalyzer) Stop(_ context.Context) error {
	if a.bn != nil {
		log := GetLogger()
		log.Info("stopping BirdNET model",
			logger.String("service", birdNETAnalyzerName))
		a.bn.Delete()
		a.bn = nil
	}
	return nil
}

// Compatible returns true if this analyzer can process audio from the given source.
// BirdNETAnalyzer handles all source types except ultrasonic (bat detection).
func (a *BirdNETAnalyzer) Compatible(source app.AudioSource) bool {
	return source.Type != app.SourceTypeUltrasonic
}

// BirdNET returns the underlying classifier orchestrator, or nil if the analyzer
// has not been started. Callers must not use the returned pointer after Stop().
func (a *BirdNETAnalyzer) BirdNET() *classifier.Orchestrator {
	return a.bn
}

