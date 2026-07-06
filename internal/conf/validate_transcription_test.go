package conf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// validateTranscriptionSettings
// ---------------------------------------------------------------------------

func TestValidateTranscriptionSettings_DisabledIsAlwaysValid(t *testing.T) {
	t.Parallel()
	// When disabled, even an empty or missing model path must pass: transcription
	// is optional and must not block startup.
	cases := []TranscriptionSettings{
		{Enabled: false},
		{Enabled: false, Model: ""},
		{Enabled: false, Model: "/no/such/model.bin", Binary: "", Language: ""},
	}
	for i := range cases {
		cfg := cases[i]
		t.Run("case", func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateTranscriptionSettings(&cfg), "case %d should pass when disabled", i)
		})
	}
}

func TestValidateTranscriptionSettings_EnabledRequiresModelPath(t *testing.T) {
	t.Parallel()
	cfg := TranscriptionSettings{Enabled: true, Model: ""}
	err := validateTranscriptionSettings(&cfg)
	require.Error(t, err, "enabled transcription with empty model must fail")
	assert.Contains(t, err.Error(), "model path", "error should mention the model path")
}

func TestValidateTranscriptionSettings_EnabledRequiresExistingModel(t *testing.T) {
	t.Parallel()
	cfg := TranscriptionSettings{Enabled: true, Model: "/definitely/not/here/ggml.bin"}
	err := validateTranscriptionSettings(&cfg)
	require.Error(t, err, "enabled transcription with missing model file must fail")
	assert.Contains(t, err.Error(), "does not exist", "error should explain the model file is missing")
}

func TestValidateTranscriptionSettings_EnabledWithExistingModelPasses(t *testing.T) {
	t.Parallel()
	// Create a real file to stand in for a GGML model so os.Stat succeeds. The
	// validator only checks existence, not the file contents.
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-tiny.en.bin")
	require.NoError(t, os.WriteFile(modelPath, []byte("fake model"), 0o600))

	cfg := TranscriptionSettings{
		Enabled:  true,
		Model:    modelPath,
		Binary:   "whisper-cli",
		Language: "en",
	}
	assert.NoError(t, validateTranscriptionSettings(&cfg), "enabled transcription with existing model must pass")
}
