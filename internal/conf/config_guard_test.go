package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quotedPath renders path the way the guard's error message does.
//
// Always use this when asserting that an error names a path. The guard formats
// paths with %q, which escapes backslashes, so on Windows a message about
// C:\Users\...\config.yaml actually reads C:\\Users\\...\\config.yaml.
// Asserting on the raw path passes on unix - where there are no backslashes to
// escape - and fails only on Windows, which is how it slipped through once.
func quotedPath(path string) string {
	return fmt.Sprintf("%q", path)
}

// realUserConfigFile returns the path the production resolver would pick for
// this machine's own config.yaml. Guard tests only ever pass this path to
// guarded entry points, which reject it before any filesystem access, so no
// test in this file writes to it.
func realUserConfigFile(t *testing.T) string {
	t.Helper()
	dir, err := userConfigDir()
	require.NoError(t, err, "the real user config dir must resolve for this guard test to be meaningful")
	return filepath.Join(dir, configFileName)
}

// requireUntouched asserts that path is in exactly the state snapshot
// described, proving a guarded call wrote nothing. A nil snapshot means the
// file did not exist beforehand and must still not exist.
func requireUntouched(t *testing.T, path string, snapshot os.FileInfo) {
	t.Helper()
	after, err := os.Stat(path)
	if snapshot == nil {
		require.Error(t, err, "guarded call must not create %s", path)
		assert.True(t, os.IsNotExist(err), "expected %s to still be absent, got: %v", path, err)
		return
	}
	require.NoError(t, err, "pre-existing %s must still be present", path)
	assert.Equal(t, snapshot.ModTime(), after.ModTime(), "guarded call must not modify %s", path)
	assert.Equal(t, snapshot.Size(), after.Size(), "guarded call must not rewrite %s", path)
}

func TestGuardRealConfigPath_RejectsUserConfigDir(t *testing.T) {
	path := realUserConfigFile(t)

	err := guardRealConfigPath(path)

	require.Error(t, err, "the guard must reject the machine owner's config file")
	require.ErrorContains(t, err, quotedPath(path), "the failure must name the path that was refused")
	require.ErrorContains(t, err, "conftest.SandboxConfigFile", "the failure must name the opt-in helper")
}

func TestGuardRealConfigPath_RejectsNestedPathUnderUserConfigDir(t *testing.T) {
	dir, err := userConfigDir()
	require.NoError(t, err)

	err = guardRealConfigPath(filepath.Join(dir, "nested", configFileName))

	require.Error(t, err, "the guard must reject anything under the real config directory, not just config.yaml")
}

func TestGuardRealConfigPath_RejectsSystemConfigDir(t *testing.T) {
	err := guardRealConfigPath(filepath.Join(systemConfigDir, configFileName))

	require.Error(t, err, "the guard must reject the system-wide config directory")
}

func TestGuardRealConfigPath_AllowsSandboxedPath(t *testing.T) {
	err := guardRealConfigPath(filepath.Join(t.TempDir(), configFileName))

	require.NoError(t, err, "a temp-dir config path is what tests are supposed to use")
}

// TestSaveYAMLConfig_RefusesRealUserConfigPath is the core regression test: it
// simulates the historical bug (a test saving settings to the resolved default
// path) and asserts the write is refused. It asserts on the guard's behaviour
// and on the real file being untouched; it never writes there itself.
func TestSaveYAMLConfig_RefusesRealUserConfigPath(t *testing.T) {
	path := realUserConfigFile(t)

	var snapshot os.FileInfo
	if info, err := os.Stat(path); err == nil {
		snapshot = info
	}

	err := SaveYAMLConfig(path, &Settings{})

	require.Error(t, err, "SaveYAMLConfig must refuse the real user config path under test")
	require.ErrorContains(t, err, "conftest.SandboxConfigFile",
		"the failure must tell the developer how to sandbox the test")
	requireUntouched(t, path, snapshot)
}

func TestSaveYAMLConfig_WritesToSandboxedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)

	require.NoError(t, SaveYAMLConfig(path, &Settings{}), "a sandboxed path must still save normally")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, data, "the sandboxed config file should contain the marshalled settings")
}

// TestFindConfigFile_ResolvesRealUserConfigPath pins the deliberate decision
// NOT to guard resolution. Reads are unguarded because guarding them broke the
// suite repo-wide: Load() would fail, conf.Setting() would return nil, and
// unrelated packages crashed on the nil snapshot. If someone re-adds a guard to
// FindConfigFile, this test fails and points them at config_guard.go.
func TestFindConfigFile_ResolvesRealUserConfigPath(t *testing.T) {
	path := realUserConfigFile(t)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no real config file at %s; nothing to resolve here", path)
	}

	origConfigPath := ConfigPath
	t.Cleanup(func() { ConfigPath = origConfigPath })
	ConfigPath = path

	resolved, err := FindConfigFile()

	require.NoError(t, err, "resolution must stay unguarded; only writes are refused")
	assert.Equal(t, path, resolved)
}

// TestSaveSettings_RefusesRealUserConfigPath exercises the full
// SaveSettings -> FindConfigFile -> SaveYAMLConfig chain that the historical
// offender used: resolution succeeds, and the write at the end is refused.
func TestSaveSettings_RefusesRealUserConfigPath(t *testing.T) {
	path := realUserConfigFile(t)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no real config file at %s; SaveSettings cannot resolve it here", path)
	}

	var snapshot os.FileInfo
	if info, err := os.Stat(path); err == nil {
		snapshot = info
	}

	origConfigPath := ConfigPath
	origSettings := settingsInstance.Load()
	t.Cleanup(func() {
		ConfigPath = origConfigPath
		StoreSettings(origSettings)
	})
	ConfigPath = path
	StoreSettings(&Settings{})

	err := SaveSettings()

	require.Error(t, err, "SaveSettings must refuse to persist into the real user config")
	require.ErrorContains(t, err, "conftest.SandboxConfigFile")
	requireUntouched(t, path, snapshot)
}
