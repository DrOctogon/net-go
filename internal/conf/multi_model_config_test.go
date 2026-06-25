package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKnownIDs mirrors classifier.KnownConfigIDs() for testing without circular imports.
var testKnownIDs = map[string]bool{"voicewatch": true, "human_voice": true}

func TestModelsConfig_Defaults(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	assert.Empty(t, settings.Models.Enabled)
}

func TestAudioSourceConfig_ModelsField(t *testing.T) {
	t.Parallel()
	src := AudioSourceConfig{
		Name:   "Test Mic",
		Device: "hw:0,0",
		Models: []string{"voicewatch", "perch_v2"},
	}
	assert.Equal(t, []string{"voicewatch", "perch_v2"}, src.Models)
}

func TestStreamConfig_ModelsField(t *testing.T) {
	t.Parallel()
	stream := StreamConfig{
		Name:   "Garden Cam",
		URL:    "rtsp://192.168.1.100/audio",
		Models: []string{"voicewatch"},
	}
	assert.Equal(t, []string{"voicewatch"}, stream.Models)
}

func TestMigrateSourceModels_SingularToPlural(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Realtime.Audio.Sources = []AudioSourceConfig{
		{Name: "Mic1", Device: "hw:0,0", Model: "perch_v2"},
	}
	migrated := settings.MigrateSourceModels()
	require.True(t, migrated)
	assert.Equal(t, []string{"perch_v2"}, settings.Realtime.Audio.Sources[0].Models)
	assert.Empty(t, settings.Realtime.Audio.Sources[0].Model, "legacy field should be cleared")
}

func TestMigrateSourceModels_DefaultToBirdNET(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Realtime.Audio.Sources = []AudioSourceConfig{
		{Name: "Mic1", Device: "hw:0,0"},
	}
	migrated := settings.MigrateSourceModels()
	require.True(t, migrated)
	assert.Equal(t, []string{"voicewatch"}, settings.Realtime.Audio.Sources[0].Models)
}

func TestMigrateSourceModels_SkipIfModelsAlreadySet(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Realtime.Audio.Sources = []AudioSourceConfig{
		{Name: "Mic1", Device: "hw:0,0", Models: []string{"voicewatch", "perch_v2"}},
	}
	migrated := settings.MigrateSourceModels()
	assert.False(t, migrated, "should not migrate if Models already set")
	assert.Equal(t, []string{"voicewatch", "perch_v2"}, settings.Realtime.Audio.Sources[0].Models)
}

func TestMigrateSourceModels_StreamConfigMigration(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Realtime.RTSP.Streams = []StreamConfig{
		{Name: "Cam1", URL: "rtsp://host/audio"},
	}
	migrated := settings.MigrateSourceModels()
	require.True(t, migrated)
	assert.Equal(t, []string{"voicewatch"}, settings.Realtime.RTSP.Streams[0].Models)
}

func TestValidateModelConfig_NoErrorsWithJustBirdNET(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Models.Enabled = []string{"voicewatch"}
	errs := settings.ValidateModelConfig(testKnownIDs, true)
	assert.Empty(t, errs, "should have no errors with just VoiceWatch")
}

func TestValidateModelConfig_UnknownModelWarning(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Models.Enabled = []string{"voicewatch", "unknown_model"}
	warnings := settings.ValidateModelConfig(testKnownIDs, true)
	assert.NotEmpty(t, warnings, "unknown model ID should produce a warning")
}

func TestValidateModelConfig_SourceReferencesUnavailableModel(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Models.Enabled = []string{"voicewatch"}
	settings.Realtime.Audio.Sources = []AudioSourceConfig{
		{Name: "Mic1", Device: "hw:0,0", Models: []string{"voicewatch", "perch_v2"}},
	}
	warnings := settings.ValidateModelConfig(testKnownIDs, true)
	assert.NotEmpty(t, warnings, "source referencing model not in models.enabled should warn")
}

func TestValidateModelConfig_SkipSourceRefsAtEarlyLoading(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.Models.Enabled = []string{"voicewatch"}
	settings.Realtime.Audio.Sources = []AudioSourceConfig{
		{Name: "Mic1", Device: "hw:0,0", Models: []string{"voicewatch", "perch_v2"}},
	}
	warnings := settings.ValidateModelConfig(testKnownIDs, false)
	assert.Empty(t, warnings, "source reference checks should be skipped when checkSourceRefs is false")
}

func TestVoiceWatchConfig_VersionField(t *testing.T) {
	t.Parallel()
	settings := &Settings{}
	settings.VoiceWatch.Version = "2.4"
	assert.Equal(t, "2.4", settings.VoiceWatch.Version)
}
