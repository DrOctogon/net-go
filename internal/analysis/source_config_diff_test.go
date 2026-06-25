package analysis

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tphakala/voicewatch/internal/audiocore"
	"github.com/tphakala/voicewatch/internal/audiocore/buffer"
	"github.com/tphakala/voicewatch/internal/classifier"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/logger"
)

func TestSourceNeedsReconfigure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		running  *audiocore.AudioSource
		desired  *audiocore.SourceConfig
		expected bool
	}{
		{
			name: "same config, no reconfigure needed",
			running: &audiocore.AudioSource{
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   1,
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   1,
			},
			expected: false,
		},
		{
			name: "sample rate changed",
			running: &audiocore.AudioSource{
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   1,
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 96000,
				BitDepth:   16,
				Channels:   1,
			},
			expected: true,
		},
		{
			name: "bit depth changed",
			running: &audiocore.AudioSource{
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   1,
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 48000,
				BitDepth:   32,
				Channels:   1,
			},
			expected: true,
		},
		{
			name: "channels changed",
			running: &audiocore.AudioSource{
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   1,
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   2,
			},
			expected: true,
		},
		{
			// Unset and explicit "downmix" produce identical FFmpeg args, so the
			// transition must not trigger a stream restart.
			name: "channel mode unset to downmix is a no-op",
			running: &audiocore.AudioSource{
				SampleRate: 48000, BitDepth: 16, Channels: 1, ChannelMode: "",
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 48000, BitDepth: 16, Channels: 1, ChannelMode: string(conf.ChannelModeDownmix),
			},
			expected: false,
		},
		{
			name: "channel mode downmix to left changes",
			running: &audiocore.AudioSource{
				SampleRate: 48000, BitDepth: 16, Channels: 2, ChannelMode: string(conf.ChannelModeDownmix),
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 48000, BitDepth: 16, Channels: 2, ChannelMode: string(conf.ChannelModeLeft),
			},
			expected: true,
		},
		{
			name: "channel mode left to right changes",
			running: &audiocore.AudioSource{
				SampleRate: 48000, BitDepth: 16, Channels: 2, ChannelMode: string(conf.ChannelModeLeft),
			},
			desired: &audiocore.SourceConfig{
				SampleRate: 48000, BitDepth: 16, Channels: 2, ChannelMode: string(conf.ChannelModeRight),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sourceNeedsReconfigure(tt.running, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// newModelTestBufferManager creates a buffer.Manager with analysis buffers allocated
// for the given (sourceID, modelID) pairs. Each buffer is allocated with
// minimal dimensions suitable for testing.
func newModelTestBufferManager(t *testing.T, pairs [][2]string) *buffer.Manager {
	t.Helper()
	mgr := buffer.NewManager(logger.NewSlogLogger(io.Discard, logger.LogLevelError, time.UTC))
	for _, p := range pairs {
		err := mgr.AllocateAnalysis(p[0], p[1], 1024, 0, 512)
		assert.NoError(t, err)
	}
	return mgr
}

func TestSourceModelsChanged(t *testing.T) {
	t.Parallel()

	const (
		src            = "rtsp_abc123"
		humanVoiceID   = classifier.RegistryIDHumanVoice
		primaryModelID = humanVoiceID
	)

	loaded := map[string]classifier.ModelInfo{
		humanVoiceID: {ID: humanVoiceID},
	}

	tests := []struct {
		name             string
		currentModels    [][2]string // (sourceID, modelID) pairs for buffer allocation
		desiredConfigIDs []string
		expected         bool
	}{
		{
			name:             "no change, single model",
			currentModels:    [][2]string{{src, humanVoiceID}},
			desiredConfigIDs: []string{conf.ModelIDHumanVoice},
			expected:         false,
		},
		{
			name:             "empty desired falls back to primary, no change",
			currentModels:    [][2]string{{src, humanVoiceID}},
			desiredConfigIDs: []string{},
			expected:         false,
		},
		{
			name:             "unknown config ID ignored, no effective change",
			currentModels:    [][2]string{{src, humanVoiceID}},
			desiredConfigIDs: []string{conf.ModelIDHumanVoice, "unknown_model"},
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := newModelTestBufferManager(t, tt.currentModels)
			result := sourceModelsChanged(mgr, src, tt.desiredConfigIDs, loaded, primaryModelID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveDesiredModelSet(t *testing.T) {
	t.Parallel()

	loaded := map[string]classifier.ModelInfo{
		classifier.RegistryIDHumanVoice: {ID: classifier.RegistryIDHumanVoice},
	}

	t.Run("resolves known loaded models", func(t *testing.T) {
		t.Parallel()
		set := resolveDesiredModelSet([]string{conf.ModelIDHumanVoice}, loaded, classifier.RegistryIDHumanVoice)
		assert.True(t, set[classifier.RegistryIDHumanVoice])
		assert.Len(t, set, 1)
	})

	t.Run("skips unknown config IDs", func(t *testing.T) {
		t.Parallel()
		set := resolveDesiredModelSet([]string{conf.ModelIDHumanVoice, "unknown"}, loaded, classifier.RegistryIDHumanVoice)
		assert.True(t, set[classifier.RegistryIDHumanVoice])
		assert.Len(t, set, 1)
	})

	t.Run("skips unloaded models", func(t *testing.T) {
		t.Parallel()
		// HumanVoice is the registered model but it is deliberately absent from the loaded map.
		// The resolved set is empty so the function falls back to the primary model ID.
		emptyLoaded := map[string]classifier.ModelInfo{}
		set := resolveDesiredModelSet([]string{conf.ModelIDHumanVoice}, emptyLoaded, classifier.RegistryIDHumanVoice)
		assert.True(t, set[classifier.RegistryIDHumanVoice], "primary fallback must be present when no model is loaded")
		assert.Len(t, set, 1)
	})

	t.Run("empty config falls back to primary", func(t *testing.T) {
		t.Parallel()
		set := resolveDesiredModelSet(nil, loaded, classifier.RegistryIDHumanVoice)
		assert.True(t, set[classifier.RegistryIDHumanVoice])
		assert.Len(t, set, 1)
	})
}

func TestSourceModelsChanged_UnloadedModelIgnored(t *testing.T) {
	t.Parallel()

	const src = "rtsp_abc123"

	// Only BirdNET is loaded; Perch is not.
	loadedOnlyBirdnet := map[string]classifier.ModelInfo{
		"BirdNET_V2.4": {ID: "BirdNET_V2.4"},
	}

	mgr := newModelTestBufferManager(t, [][2]string{{src, "BirdNET_V2.4"}})

	// Config requests perch_v2 but it's not loaded: should NOT report a
	// change so we avoid a spurious rebuild on every hot-reload tick.
	changed := sourceModelsChanged(mgr, src, []string{"birdnet", "perch_v2"}, loadedOnlyBirdnet, "BirdNET_V2.4")
	assert.False(t, changed, "unloaded model in desired config should be ignored")
}
