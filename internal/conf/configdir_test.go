package conf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTokensArtifact = "tokens.json"
	testKeyArtifact    = "encryption.key"
	testSessionsDir    = "sessions"
	testStateArtifact  = "backup-state.json"
)

// setConfigPath points the package-level --config override at path for the
// duration of the test. ConfigPath is global mutable state, so tests using this
// helper must not run in parallel.
func setConfigPath(t *testing.T, path string) {
	t.Helper()
	previous := ConfigPath
	t.Cleanup(func() { ConfigPath = previous })
	ConfigPath = path
}

// TestConfigArtifactPathHonorsConfigFlag is the core regression test: every
// sibling artifact must be written beside the --config file rather than in the
// OS default directory. This is what tokens, sessions, backup state and the
// backup encryption key all got wrong.
func TestConfigArtifactPathHonorsConfigFlag(t *testing.T) {
	scratch := t.TempDir()
	configFile := filepath.Join(scratch, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("debug: false\n"), 0o600))
	setConfigPath(t, configFile)

	for _, name := range []string{testTokensArtifact, testKeyArtifact, testSessionsDir, testStateArtifact} {
		got, err := ConfigArtifactPath(name)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(scratch, name), got,
			"%s must be co-located with the --config file", name)
	}
}

// TestConfigArtifactPathHonorsConfigFlagBeforeFirstSave covers a fresh install
// or a fresh container mount: --config names a file that does not exist yet.
func TestConfigArtifactPathHonorsConfigFlagBeforeFirstSave(t *testing.T) {
	scratch := t.TempDir()
	setConfigPath(t, filepath.Join(scratch, "config.yaml"))

	got, err := ConfigArtifactPath(testTokensArtifact)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(scratch, testTokensArtifact), got)
}

// TestConfigArtifactPathDefaultPathUnchanged pins requirement 2: with no
// --config flag the artifact resolves into a directory the pre-fix code would
// also have used, so nothing moves for a default-path install.
//
// This test only computes paths - it never creates or writes anything - so it is
// safe to run against the real default config directory.
func TestConfigArtifactPathDefaultPathUnchanged(t *testing.T) {
	setConfigPath(t, "")

	defaultPaths, err := GetDefaultConfigPaths()
	require.NoError(t, err)
	require.NotEmpty(t, defaultPaths)

	got, err := ConfigArtifactPath(testTokensArtifact)
	require.NoError(t, err)

	// Accept the pre-fix location, or the directory of the config.yaml the
	// default search/viper already resolved - for a default install these are the
	// same directory. What must never happen is the path escaping that set.
	allowed := make([]string, 0, len(defaultPaths)+1)
	for _, dir := range defaultPaths {
		allowed = append(allowed, filepath.Join(dir, testTokensArtifact))
	}
	if active, err := FindConfigFile(); err == nil {
		allowed = append(allowed, filepath.Join(filepath.Dir(active), testTokensArtifact))
	}
	assert.Contains(t, allowed, got,
		"a default-path install must resolve where it did before this fix")
}

// TestLegacyArtifactFallback exercises the migration decision against temporary
// directories. GetUserHomeDir is process-cached and prefers /etc/passwd over
// $HOME, so the real default directory cannot be redirected; the fallback logic
// is therefore tested directly with an injected legacy directory.
func TestLegacyArtifactFallback(t *testing.T) {
	t.Run("finds a pre-existing legacy file", func(t *testing.T) {
		t.Parallel()
		// The migration hazard: for the backup encryption key, failing to find a
		// pre-existing key means permanently undecryptable backups.
		legacyDir, activeDir := t.TempDir(), t.TempDir()
		legacyKey := filepath.Join(legacyDir, testKeyArtifact)
		require.NoError(t, os.WriteFile(legacyKey, []byte("deadbeef"), 0o600))

		got := legacyArtifactFallback(testKeyArtifact, filepath.Join(activeDir, testKeyArtifact), legacyDir)
		assert.Equal(t, legacyKey, got, "a pre-existing key must stay reachable")
	})

	t.Run("active copy wins once it exists", func(t *testing.T) {
		t.Parallel()
		legacyDir, activeDir := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(legacyDir, testTokensArtifact), []byte("{}"), 0o600))
		activePath := filepath.Join(activeDir, testTokensArtifact)
		require.NoError(t, os.WriteFile(activePath, []byte("{}"), 0o600))

		assert.Empty(t, legacyArtifactFallback(testTokensArtifact, activePath, legacyDir),
			"a completed migration must never be undone")
	})

	t.Run("no fallback when the legacy directory is empty", func(t *testing.T) {
		t.Parallel()
		// The container case: an empty default directory means no migration.
		legacyDir, activeDir := t.TempDir(), t.TempDir()
		assert.Empty(t, legacyArtifactFallback(testTokensArtifact,
			filepath.Join(activeDir, testTokensArtifact), legacyDir))
	})

	t.Run("no fallback for a default-path install", func(t *testing.T) {
		t.Parallel()
		// active == legacy, so there is nothing to migrate and behaviour is
		// byte-for-byte what it was before this fix.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, testKeyArtifact), []byte("deadbeef"), 0o600))

		assert.Empty(t, legacyArtifactFallback(testKeyArtifact, filepath.Join(dir, testKeyArtifact), dir))
	})

	t.Run("ignores an empty legacy directory artifact", func(t *testing.T) {
		t.Parallel()
		// The session store is routinely created empty when store init fails and
		// falls back to cookies; an empty directory is not data worth pinning to.
		legacyDir, activeDir := t.TempDir(), t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(legacyDir, testSessionsDir), 0o750))

		assert.Empty(t, legacyArtifactFallback(testSessionsDir,
			filepath.Join(activeDir, testSessionsDir), legacyDir))
	})

	t.Run("finds a non-empty legacy directory artifact", func(t *testing.T) {
		t.Parallel()
		legacyDir, activeDir := t.TempDir(), t.TempDir()
		legacySessions := filepath.Join(legacyDir, testSessionsDir)
		require.NoError(t, os.MkdirAll(legacySessions, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(legacySessions, "session_abc"), []byte("x"), 0o600))

		assert.Equal(t, legacySessions, legacyArtifactFallback(testSessionsDir,
			filepath.Join(activeDir, testSessionsDir), legacyDir))
	})

	t.Run("an empty active directory does not count as the artifact", func(t *testing.T) {
		t.Parallel()
		// Guards the ordering trap: MkdirAll on the active session path must not
		// make the fallback vanish before the legacy sessions have been found.
		legacyDir, activeDir := t.TempDir(), t.TempDir()
		legacySessions := filepath.Join(legacyDir, testSessionsDir)
		require.NoError(t, os.MkdirAll(legacySessions, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(legacySessions, "session_abc"), []byte("x"), 0o600))
		activeSessions := filepath.Join(activeDir, testSessionsDir)
		require.NoError(t, os.MkdirAll(activeSessions, 0o750))

		assert.Equal(t, legacySessions, legacyArtifactFallback(testSessionsDir, activeSessions, legacyDir))
	})
}

// TestResolveConfigArtifactReadInPlace covers the read-in-place variant used for
// the backup encryption key: one path for both reads and writes, so the key's
// readers and writers can never disagree.
func TestResolveConfigArtifactReadInPlace(t *testing.T) {
	scratch := t.TempDir()
	setConfigPath(t, filepath.Join(scratch, "config.yaml"))

	// Nothing in the legacy location -> the artifact goes beside --config.
	got, err := ResolveConfigArtifact(testKeyArtifact)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(scratch, testKeyArtifact), got)
}
