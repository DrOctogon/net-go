package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/errors"
)

// ---------------------------------------------------------------------------
// validateContinuousRecordingSettings
// ---------------------------------------------------------------------------

func TestValidateContinuousRecordingSettings_DisabledIsAlwaysValid(t *testing.T) {
	t.Parallel()
	// Disabled with zero-value or deliberately bad fields must still pass.
	cases := []ContinuousRecordingSettings{
		{Enabled: false},
		{Enabled: false, SegmentSeconds: 0, RetentionHours: 0, Format: "", SampleRate: -1},
		{Enabled: false, SegmentSeconds: -99, RetentionHours: -99, Format: "invalid", SampleRate: -99},
	}
	for i, cfg := range cases {
		cfg := cfg
		t.Run("case", func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateContinuousRecordingSettings(&cfg), "case %d should pass when disabled", i)
		})
	}
}

func TestValidateContinuousRecordingSettings_ValidEnabled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  ContinuousRecordingSettings
	}{
		{
			name: "flac with default sample rate",
			cfg: ContinuousRecordingSettings{
				Enabled: true, Path: "recordings/",
				SegmentSeconds: 3600, RetentionHours: 24,
				Format: "flac", SampleRate: 0,
			},
		},
		{
			name: "wav with explicit sample rate",
			cfg: ContinuousRecordingSettings{
				Enabled: true, Path: "recordings/",
				SegmentSeconds: 900, RetentionHours: 48,
				Format: "wav", SampleRate: 48000,
			},
		},
		{
			name: "minimum positive values",
			cfg: ContinuousRecordingSettings{
				Enabled: true, Path: "r/",
				SegmentSeconds: 1, RetentionHours: 1,
				Format: "flac", SampleRate: 0,
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateContinuousRecordingSettings(&tc.cfg))
		})
	}
}

func TestValidateContinuousRecordingSettings_SegmentSecondsZeroFails(t *testing.T) {
	t.Parallel()
	cfg := ContinuousRecordingSettings{
		Enabled: true, SegmentSeconds: 0, RetentionHours: 24, Format: "flac",
	}
	err := validateContinuousRecordingSettings(&cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "segmentSeconds")

	enhanced, ok := err.(*errors.EnhancedError) //nolint:errorlint // direct assertion for test inspection
	require.True(t, ok, "expected *errors.EnhancedError")
	assert.Equal(t, errors.CategoryValidation, enhanced.Category)
	ctx := enhanced.GetContext()
	assert.Equal(t, "continuous-recording-segment-seconds", ctx["validation_type"])
}

func TestValidateContinuousRecordingSettings_SegmentSecondsNegativeFails(t *testing.T) {
	t.Parallel()
	cfg := ContinuousRecordingSettings{
		Enabled: true, SegmentSeconds: -1, RetentionHours: 24, Format: "flac",
	}
	err := validateContinuousRecordingSettings(&cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "segmentSeconds")
}

func TestValidateContinuousRecordingSettings_RetentionHoursZeroFails(t *testing.T) {
	t.Parallel()
	cfg := ContinuousRecordingSettings{
		Enabled: true, SegmentSeconds: 3600, RetentionHours: 0, Format: "flac",
	}
	err := validateContinuousRecordingSettings(&cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retentionHours")

	enhanced, ok := err.(*errors.EnhancedError) //nolint:errorlint // direct assertion for test inspection
	require.True(t, ok, "expected *errors.EnhancedError")
	assert.Equal(t, errors.CategoryValidation, enhanced.Category)
}

func TestValidateContinuousRecordingSettings_RetentionHoursNegativeFails(t *testing.T) {
	t.Parallel()
	cfg := ContinuousRecordingSettings{
		Enabled: true, SegmentSeconds: 3600, RetentionHours: -5, Format: "flac",
	}
	err := validateContinuousRecordingSettings(&cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retentionHours")
}

func TestValidateContinuousRecordingSettings_InvalidFormatFails(t *testing.T) {
	t.Parallel()
	invalidFormats := []string{"mp3", "opus", "aac", "ogg", "", "WAV", "FLAC"}
	for _, fmt := range invalidFormats {
		fmt := fmt
		t.Run("format_"+fmt, func(t *testing.T) {
			t.Parallel()
			cfg := ContinuousRecordingSettings{
				Enabled: true, SegmentSeconds: 3600, RetentionHours: 24, Format: fmt,
			}
			err := validateContinuousRecordingSettings(&cfg)
			require.Error(t, err, "format %q should be rejected", fmt)
			assert.Contains(t, err.Error(), "format")
		})
	}
}

func TestValidateContinuousRecordingSettings_ValidFormats(t *testing.T) {
	t.Parallel()
	for _, fmt := range []string{"flac", "wav"} {
		fmt := fmt
		t.Run("format_"+fmt, func(t *testing.T) {
			t.Parallel()
			cfg := ContinuousRecordingSettings{
				Enabled: true, SegmentSeconds: 3600, RetentionHours: 24, Format: fmt, SampleRate: 0,
			}
			assert.NoError(t, validateContinuousRecordingSettings(&cfg))
		})
	}
}

func TestValidateContinuousRecordingSettings_NegativeSampleRateFails(t *testing.T) {
	t.Parallel()
	cfg := ContinuousRecordingSettings{
		Enabled: true, SegmentSeconds: 3600, RetentionHours: 24, Format: "flac", SampleRate: -1,
	}
	err := validateContinuousRecordingSettings(&cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sampleRate")

	enhanced, ok := err.(*errors.EnhancedError) //nolint:errorlint // direct assertion for test inspection
	require.True(t, ok, "expected *errors.EnhancedError")
	assert.Equal(t, errors.CategoryValidation, enhanced.Category)
}

func TestValidateContinuousRecordingSettings_ZeroSampleRateIsValid(t *testing.T) {
	t.Parallel()
	cfg := ContinuousRecordingSettings{
		Enabled: true, SegmentSeconds: 3600, RetentionHours: 24, Format: "flac", SampleRate: 0,
	}
	assert.NoError(t, validateContinuousRecordingSettings(&cfg))
}
