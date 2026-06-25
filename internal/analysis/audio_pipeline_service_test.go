package analysis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/app"
	"github.com/tphakala/voicewatch/internal/audiocore"
	"github.com/tphakala/voicewatch/internal/classifier"
	"github.com/tphakala/voicewatch/internal/conf"
)

// Compile-time interface compliance check.
var _ app.Service = (*AudioPipelineService)(nil)

func TestAudioPipelineService_Name(t *testing.T) {
	t.Parallel()

	svc := NewAudioPipelineService(&conf.Settings{}, nil, nil, nil, nil)
	assert.Equal(t, "audio-pipeline", svc.Name())
}

func TestAudioPipelineService_Stop_NilSafe(t *testing.T) {
	t.Parallel()

	svc := NewAudioPipelineService(&conf.Settings{}, nil, nil, nil, nil)
	// Stop before Start should not panic and should return nil.
	assert.NotPanics(t, func() {
		err := svc.Stop(t.Context())
		assert.NoError(t, err)
	})
}

func TestResolveModelTargets_EmptyInput(t *testing.T) {
	t.Parallel()

	loaded := map[string]classifier.ModelInfo{
		"BirdNET_V2.4": {ID: "BirdNET_V2.4", Spec: classifier.ModelSpec{SampleRate: 48000}},
	}
	targets := resolveModelTargets(nil, loaded)
	assert.Empty(t, targets, "nil config IDs should return nil")

	targets = resolveModelTargets([]string{}, loaded)
	assert.Empty(t, targets, "empty config IDs should return empty")
}

func TestResolveModelTargets_SingleModel(t *testing.T) {
	t.Parallel()

	loaded := map[string]classifier.ModelInfo{
		classifier.RegistryIDHumanVoice: {
			ID:   classifier.RegistryIDHumanVoice,
			Name: "Human Voice",
			Spec: classifier.ModelSpec{SampleRate: 16000, ClipLength: 3 * time.Second},
		},
	}

	targets := resolveModelTargets([]string{conf.ModelIDHumanVoice}, loaded)
	require.Len(t, targets, 1)
	assert.Equal(t, classifier.RegistryIDHumanVoice, targets[0].ID)
	assert.Equal(t, 16000, targets[0].Spec.SampleRate)
}

func TestResolveModelTargets_UnknownConfigID(t *testing.T) {
	t.Parallel()

	loaded := map[string]classifier.ModelInfo{
		"BirdNET_V2.4": {
			ID:   "BirdNET_V2.4",
			Spec: classifier.ModelSpec{SampleRate: 48000},
		},
	}

	// "unknown_model" is not in configToRegistryID, so it should be skipped.
	targets := resolveModelTargets([]string{"unknown_model"}, loaded)
	assert.Empty(t, targets)
}

func TestResolveModelTargets_KnownButNotLoaded(t *testing.T) {
	t.Parallel()

	// human_voice resolves to classifier.RegistryIDHumanVoice but is not in the loaded models map.
	loaded := map[string]classifier.ModelInfo{}

	targets := resolveModelTargets([]string{conf.ModelIDHumanVoice}, loaded)
	assert.Empty(t, targets, "human_voice resolves to HumanVoice but HumanVoice is not loaded")
}

func TestBuildLivenessConfig_AllDefaults(t *testing.T) {
	t.Parallel()

	cfg := buildLivenessConfig(conf.WatchdogSettings{})
	defaults := audiocore.DefaultLivenessConfig()

	assert.Equal(t, defaults.CheckInterval, cfg.CheckInterval)
	assert.Equal(t, defaults.SilenceThreshold, cfg.SilenceThreshold)
	assert.Equal(t, defaults.MaxRetries, cfg.MaxRetries)
	assert.Equal(t, defaults.RetryBackoff, cfg.RetryBackoff)
	assert.Equal(t, defaults.CooldownAfterRecov, cfg.CooldownAfterRecov)
	assert.Equal(t, defaults.EscalationTimeout, cfg.EscalationTimeout)
}

func TestBuildLivenessConfig_CustomValues(t *testing.T) {
	t.Parallel()

	ws := conf.WatchdogSettings{
		CheckInterval:     5,
		SilenceThreshold:  60,
		MaxRetries:        5,
		RetryBackoff:      10,
		Cooldown:          120,
		EscalationTimeout: 90,
	}
	cfg := buildLivenessConfig(ws)

	assert.Equal(t, 5*time.Second, cfg.CheckInterval)
	assert.Equal(t, 60*time.Second, cfg.SilenceThreshold)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 10*time.Second, cfg.RetryBackoff)
	assert.Equal(t, 120*time.Second, cfg.CooldownAfterRecov)
	assert.Equal(t, 90*time.Second, cfg.EscalationTimeout)
}

func TestBuildLivenessConfig_PartialOverride(t *testing.T) {
	t.Parallel()

	ws := conf.WatchdogSettings{
		SilenceThreshold: 45,
		MaxRetries:       10,
	}
	cfg := buildLivenessConfig(ws)
	defaults := audiocore.DefaultLivenessConfig()

	assert.Equal(t, defaults.CheckInterval, cfg.CheckInterval, "unset field should use default")
	assert.Equal(t, 45*time.Second, cfg.SilenceThreshold)
	assert.Equal(t, 10, cfg.MaxRetries)
	assert.Equal(t, defaults.RetryBackoff, cfg.RetryBackoff, "unset field should use default")
	assert.Equal(t, defaults.CooldownAfterRecov, cfg.CooldownAfterRecov, "unset field should use default")
	assert.Equal(t, defaults.EscalationTimeout, cfg.EscalationTimeout, "unset field should use default")
}
