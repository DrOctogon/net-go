// Package humanvoice implements human voice / speech detection as a
// classifier.ModelInstance backed by a Silero VAD ONNX model. It co-runs
// alongside the other audio models: clips arrive at the model's native 16 kHz
// rate, the VAD backend produces per-frame speech probabilities, and each clip
// is aggregated into a single "Human Voice" detection.
//
// Phase 1 (additive) provides the ModelInstance implementation, registry, and
// config plumbing. The orchestrator load path is wired in a later phase.
package humanvoice

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/tphakala/voicewatch/internal/classifier"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/inference"
	"github.com/tphakala/voicewatch/internal/logger"
)

// Model audio and identity constants. No magic numbers: every literal the model
// depends on is named here.
const (
	// sampleRateHz is the native input sample rate the Silero VAD model expects.
	sampleRateHz = 16000
	// clipLength is the analysis window length, matched to the other models so
	// the human voice detector can co-run on the shared analysis buffers.
	clipLength = 3 * time.Second
	// labelHumanVoice is the single class label this model emits.
	labelHumanVoice = "Human Voice"
	// numSpeciesHumanVoice is the model's class count (speech vs. not-speech is
	// collapsed into the single positive class).
	numSpeciesHumanVoice = 1
	// sileroFrameSize is the Silero VAD input frame size in samples at 16 kHz.
	// Retained as a named constant for the inference wiring completed in Phase 2.
	sileroFrameSize = 512
	// deviceCPU is the compute device reported by RuntimeInfo for the ONNX
	// Runtime CPU execution provider.
	deviceCPU = "CPU"
	// moduleName scopes this package's structured logs.
	moduleName = "birdnet"
)

// Config holds the parameters needed to construct a human voice Model.
type Config struct {
	ModelPath       string  // path to the Silero VAD ONNX model file (required)
	ONNXRuntimePath string  // path to the ONNX Runtime shared library ("" = auto-discover)
	Threads         int     // CPU threads for inference (0 = runtime default)
	Threshold       float64 // confidence threshold (applied by the processor, not here)
}

// Model is a loaded human voice / speech detector. It implements
// classifier.ModelInstance and is goroutine-safe via an internal mutex.
type Model struct {
	vad       inference.Classifier
	info      classifier.ModelInfo
	mu        sync.Mutex
	device    string
	backend   string
	precision string
}

// Compile-time assertion that Model satisfies the ModelInstance contract.
var _ classifier.ModelInstance = (*Model)(nil)

// modelSpec returns the fixed audio requirements for the human voice model,
// built from the package constants so the spec is self-consistent regardless of
// registry state.
func modelSpec() classifier.ModelSpec {
	return classifier.ModelSpec{SampleRate: sampleRateHz, ClipLength: clipLength}
}

// New creates a human voice Model. It fails closed with a wrapped error when the
// configured Silero VAD model file is missing (it may not be shipped yet) or
// when the ONNX Runtime / classifier cannot be initialized.
func New(cfg *Config) (*Model, error) {
	if cfg == nil {
		return nil, errors.Newf("humanvoice: nil config").
			Category(errors.CategoryValidation).
			Build()
	}

	// Fail closed when the model binary is absent. os.Stat surfaces a clear,
	// wrapped file-IO error instead of a confusing downstream ONNX failure.
	if _, statErr := os.Stat(cfg.ModelPath); statErr != nil {
		return nil, errors.New(statErr).
			Category(errors.CategoryModelInit).
			Context("model_path", cfg.ModelPath).
			Build()
	}

	if err := inference.InitONNXRuntime(cfg.ONNXRuntimePath); err != nil {
		return nil, errors.New(err).
			Category(errors.CategoryModelInit).
			Context("onnx_runtime_path", cfg.ONNXRuntimePath).
			Build()
	}

	// The Silero VAD runs stateful per-frame inference (512-sample windows at
	// 16 kHz with a recurrent state) and emits one speech probability per frame;
	// aggregateClip collapses those into the single "Human Voice" class.
	vad, err := inference.NewSileroVAD(cfg.ModelPath, inference.SileroVADOptions{
		FrameSize:  sileroFrameSize,
		SampleRate: sampleRateHz,
		Threads:    cfg.Threads,
	})
	if err != nil {
		return nil, errors.New(err).
			Category(errors.CategoryModelInit).
			Context("model_path", cfg.ModelPath).
			Build()
	}

	info := classifier.ModelRegistry[classifier.RegistryIDHumanVoice]
	info.Spec = modelSpec()

	logger.Global().Module(moduleName).Info("Human voice model initialized",
		logger.String("model_path", cfg.ModelPath))

	return &Model{
		vad:       vad,
		info:      info,
		device:    deviceCPU,
		backend:   classifier.BackendONNX,
		precision: string(classifier.QuantizationUnknown),
	}, nil
}

// Predict runs VAD inference on each clip and aggregates per-frame speech
// probabilities into a single "Human Voice" result per non-empty clip. Empty
// input yields no results and no error. The processor applies the confidence
// threshold downstream, so every aggregated clip is returned.
func (m *Model) Predict(_ context.Context, samples [][]float32) ([]datastore.Results, error) {
	if len(samples) == 0 {
		return nil, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.vad == nil {
		return nil, errors.Newf("humanvoice: classifier is not initialized").
			Category(errors.CategoryModelInit).
			Build()
	}

	results := make([]datastore.Results, 0, len(samples))
	for i := range samples {
		clip := samples[i]
		if len(clip) == 0 {
			continue
		}
		frameProbs, err := m.vad.Predict(clip)
		if err != nil {
			return nil, errors.New(err).
				Category(errors.CategoryAudio).
				Context("model", classifier.RegistryIDHumanVoice).
				Build()
		}
		results = append(results, aggregateClip(frameProbs))
	}
	return results, nil
}

// Spec returns the model's fixed audio requirements (16 kHz, 3 s clips).
func (m *Model) Spec() classifier.ModelSpec { return modelSpec() }

// ModelID returns the unique registry identifier for the human voice model.
func (m *Model) ModelID() string { return classifier.RegistryIDHumanVoice }

// ModelName returns the human-readable model name.
func (m *Model) ModelName() string {
	if m.info.Name != "" {
		return m.info.Name
	}
	return "Human Voice Detector"
}

// ModelVersion returns the model version string.
func (m *Model) ModelVersion() string {
	if m.info.DetectionVersion != "" {
		return m.info.DetectionVersion
	}
	return "1.0"
}

// NumSpecies returns the number of classes (always one: human voice).
func (m *Model) NumSpecies() int { return numSpeciesHumanVoice }

// Labels returns a fresh copy of the single-class label list.
func (m *Model) Labels() []string { return []string{labelHumanVoice} }

// RuntimeInfo returns the compute device, execution backend, and effective
// precision the model bound to at construction. All three are set once and never
// mutated, so no lock is needed. Implements ModelInstance.
func (m *Model) RuntimeInfo() (device, backend, precision string) {
	return m.device, m.backend, m.precision
}

// Close releases resources held by the model.
func (m *Model) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vad != nil {
		m.vad.Close()
		m.vad = nil
	}
	return nil
}
