package conftest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/conf"
)

// Not parallel: SandboxConfigFile mutates the process-global conf.ConfigPath.
func TestSandboxConfigFile_RedirectsResolution(t *testing.T) {
	path := SandboxConfigFile(t)

	require.FileExists(t, path, "the sandboxed config file must be seeded so resolution can stat it")
	assert.Equal(t, path, conf.ConfigPath, "SandboxConfigFile must set the explicit config path")

	resolved, err := conf.FindConfigFile()
	require.NoError(t, err, "resolution must point at the sandbox, not the machine's config")
	assert.Equal(t, path, resolved)

	dir, err := conf.ResolveConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Dir(path), dir)
}

// Not parallel: mutates conf.ConfigPath and the global settings snapshot.
func TestSandboxConfigFile_SaveSettingsWritesToSandbox(t *testing.T) {
	prevSettings := conf.GetSettings()
	t.Cleanup(func() { SetTestSettings(prevSettings) })

	settings := NewTestSettings().Build()
	settings.Main.Name = "sandbox-roundtrip"
	SetTestSettings(settings)

	path := SandboxConfigFile(t)
	require.NoError(t, conf.SaveSettings(), "an opted-in test must be able to save settings")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "sandbox-roundtrip", "settings must land in the sandboxed file")
}

// Not parallel: mutates conf.ConfigPath.
func TestSandboxConfigFile_RestoresPreviousConfigPath(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "outer.yaml")
	prev := conf.ConfigPath
	conf.ConfigPath = sentinel
	t.Cleanup(func() { conf.ConfigPath = prev })

	t.Run("inner", func(t *testing.T) {
		SandboxConfigFile(t)
		assert.NotEqual(t, sentinel, conf.ConfigPath)
	})

	assert.Equal(t, sentinel, conf.ConfigPath, "cleanup must restore the caller's conf.ConfigPath")
}
