package humanvoice

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedModel_IsValidONNX asserts the embedded Silero VAD model is present
// and carries an ONNX protobuf payload (non-empty, plausible size).
func TestEmbeddedModel_IsValidONNX(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, embeddedModel, "embedded Silero VAD model must not be empty")
	// The v5 Silero VAD model is ~2.3 MB; guard against an accidentally truncated
	// or placeholder file without pinning an exact byte count.
	assert.Greater(t, len(embeddedModel), 1<<20, "embedded model is implausibly small")
}

// TestWriteEmbeddedModel_WritesAndIsIdempotent verifies the model is extracted
// to the target directory and that a second call is a no-op (no rewrite).
func TestWriteEmbeddedModel_WritesAndIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	path, err := WriteEmbeddedModel(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, EmbeddedModelName), path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, int64(len(embeddedModel)), info.Size(), "written size must match embedded size")

	firstModTime := info.ModTime()

	// Second call must detect the existing same-size file and not rewrite it.
	path2, err := WriteEmbeddedModel(dir)
	require.NoError(t, err)
	assert.Equal(t, path, path2)

	info2, err := os.Stat(path2)
	require.NoError(t, err)
	assert.Equal(t, firstModTime, info2.ModTime(), "idempotent call must not rewrite the file")
}

// TestWriteEmbeddedModel_RewritesOnSizeMismatch verifies a corrupt/short file at
// the target path is replaced with the embedded model.
func TestWriteEmbeddedModel_RewritesOnSizeMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, EmbeddedModelName)
	require.NoError(t, os.WriteFile(path, []byte("truncated"), 0o644))

	got, err := WriteEmbeddedModel(dir)
	require.NoError(t, err)
	assert.Equal(t, path, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, int64(len(embeddedModel)), info.Size(), "short file must be replaced with the full model")
}

// TestWriteEmbeddedModel_EmptyDir returns an error for an empty directory.
func TestWriteEmbeddedModel_EmptyDir(t *testing.T) {
	t.Parallel()
	_, err := WriteEmbeddedModel("")
	require.Error(t, err)
}
