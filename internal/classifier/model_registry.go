// model_registry.go contains the single source of truth for supported models.
//
// Human-voice pivot: the registry has been reduced to the single Silero VAD
// "Human Voice" model. The multi-model bird registry (BirdNET/Perch/Bat/BSG),
// filename/version resolution, and quantization detection were removed with the
// classifier brain swap.
package classifier

import (
	"strings"
	"time"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/detection"
)

// Inference backend identifiers. BackendTFLite/BackendONNX are static model
// file-type metadata; BackendOpenVINO is a live execution provider reported by
// ModelInstance.RuntimeInfo(), never a file type.
const (
	BackendTFLite   = "TFLite"
	BackendONNX     = "ONNX"
	BackendOpenVINO = "OpenVINO"
)

// Quantization is the numeric precision of a model's weights, orthogonal to Backend.
type Quantization string

const (
	QuantizationUnknown Quantization = ""     // unspecified / not applicable
	QuantizationFP32    Quantization = "FP32" // 32-bit float
	QuantizationFP16    Quantization = "FP16" // 16-bit float
	QuantizationINT8    Quantization = "INT8" // 8-bit integer quantized
)

// RegistryIDHumanVoice is the registry ID of the single supported model.
const RegistryIDHumanVoice = "HumanVoice"

// ModelInfo represents metadata about a classifier model.
type ModelInfo struct {
	ID               string       // Unique registry identifier
	Name             string       // User-friendly name
	Backend          string       // Inference backend: "TFLite" or "ONNX"
	DetectionName    string       // Database model name
	DetectionVersion string       // Database model version
	Description      string       // Description of the model
	Spec             ModelSpec    // Audio requirements (sample rate, clip length)
	ConfigAliases    []string     // User-facing config IDs
	SupportedLocales []string     // List of supported locale codes
	DefaultLocale    string       // Default locale if none is specified
	NumSpecies       int          // Number of classes in the model
	CustomPath       string       // Path to custom model file, if any
	Quantization     Quantization // Precision of the loaded weights
	// IsStock marks the auto-resolved built-in default model.
	IsStock bool
}

// DisplayName returns the user-facing name including the backend type.
func (m *ModelInfo) DisplayName() string {
	if m.Backend == "" {
		return m.Name
	}
	return m.Name + " (" + m.Backend + ")"
}

// ModelRegistry is the single source of truth for all supported models. After
// the human-voice pivot it holds exactly one entry.
var ModelRegistry = map[string]ModelInfo{
	RegistryIDHumanVoice: {
		ID:               RegistryIDHumanVoice,
		Name:             "Human Voice Detector",
		Backend:          BackendONNX,
		DetectionName:    "HumanVoice",
		DetectionVersion: "1.0",
		Description:      "Human voice / speech detection (Silero VAD)",
		Spec:             ModelSpec{SampleRate: 16000, ClipLength: 3 * time.Second},
		ConfigAliases:    []string{conf.ModelIDHumanVoice},
		NumSpecies:       1,
	},
}

// KnownConfigIDs collects all ConfigAliases from the registry.
// Exported for conf.ValidateModelConfig to use without importing classifier.
func KnownConfigIDs() map[string]bool {
	ids := make(map[string]bool, len(ModelRegistry))
	for id := range ModelRegistry {
		for _, alias := range ModelRegistry[id].ConfigAliases {
			ids[strings.ToLower(alias)] = true
		}
	}
	return ids
}

// ConfigAliasForRegistry returns the primary config alias for a registry ID.
func ConfigAliasForRegistry(registryID string) string {
	info, ok := ModelRegistry[registryID]
	if !ok || len(info.ConfigAliases) == 0 {
		return ""
	}
	return info.ConfigAliases[0]
}

// GetModelSpec returns the ModelSpec for a registry ID.
func GetModelSpec(registryID string) (ModelSpec, bool) {
	info, ok := ModelRegistry[registryID]
	if !ok {
		return ModelSpec{}, false
	}
	return info.Spec, true
}

// ResolveConfigModelID maps a user-facing config model ID (e.g. "human_voice")
// to the internal registry ID. Case-insensitive. Returns the registry ID and
// true if found.
func ResolveConfigModelID(configID string) (string, bool) {
	for registryID := range ModelRegistry {
		for _, alias := range ModelRegistry[registryID].ConfigAliases {
			if strings.EqualFold(alias, configID) {
				return registryID, true
			}
		}
	}
	return "", false
}

// ToDetectionModelInfo converts classifier model metadata to the detection
// domain ModelInfo used for database storage in the ai_models table.
func (m *ModelInfo) ToDetectionModelInfo() detection.ModelInfo {
	if m.DetectionName == "" {
		return detection.DefaultModelInfo()
	}
	variant := "default"
	var classifierPath *string
	if m.CustomPath != "" {
		classifierPath = &m.CustomPath
		if !m.IsStock {
			variant = "custom"
		}
	}
	return detection.ModelInfo{
		Name:           m.DetectionName,
		Version:        m.DetectionVersion,
		Variant:        variant,
		ClassifierPath: classifierPath,
	}
}

// DetectionModelInfoForID returns the detection.ModelInfo for a registry model ID.
func DetectionModelInfoForID(modelID string) detection.ModelInfo {
	if info, ok := ModelRegistry[modelID]; ok {
		return info.ToDetectionModelInfo()
	}
	return detection.DefaultModelInfo()
}
