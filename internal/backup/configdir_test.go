package backup

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
)

// useConfigFlag points conf.ConfigPath at a config.yaml inside a temp directory,
// so path resolution exercises the --config branch and everything it resolves
// stays under t.TempDir() rather than the real ~/.config/birdnet-go.
//
// conf.ConfigPath is process-global, so tests using this must not be parallel.
func useConfigFlag(t *testing.T) string {
	t.Helper()

	scratch := t.TempDir()
	configFile := filepath.Join(scratch, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("debug: false\n"), 0o600))

	previous := conf.ConfigPath
	t.Cleanup(func() { conf.ConfigPath = previous })
	conf.ConfigPath = configFile

	return scratch
}

// TestEncryptionKeyPathFollowsConfigFlag is the regression test for the reported
// bug: the backup encryption key must live beside the active config file, not in
// the OS default config directory where two --config instances would share it.
func TestEncryptionKeyPathFollowsConfigFlag(t *testing.T) {
	scratch := useConfigFlag(t)

	m := &Manager{config: &conf.BackupConfig{Encryption: true}, logger: GetLogger()}
	keyPath, err := m.getEncryptionKeyPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(scratch, EncryptionKeyFileName), keyPath,
		"encryption.key must be co-located with the --config file")
}

// TestEncryptionKeyGeneratedBesideConfig checks the write path end to end: a key
// generated under --config lands in the scratch directory and reads back.
func TestEncryptionKeyGeneratedBesideConfig(t *testing.T) {
	scratch := useConfigFlag(t)

	m := &Manager{config: &conf.BackupConfig{Encryption: true}, logger: GetLogger()}
	key, err := m.getEncryptionKey()
	require.NoError(t, err)
	require.Len(t, key, AES256KeySize)

	keyFile := filepath.Join(scratch, EncryptionKeyFileName)
	require.FileExists(t, keyFile, "a freshly generated key must be written beside --config")

	// Reading again must return the same key, not generate a second one.
	again, err := m.getEncryptionKey()
	require.NoError(t, err)
	assert.Equal(t, key, again)
}

// TestEncryptionKeyNeverRegeneratedOverExistingKey is the migration hazard at
// this site: whatever path the resolver hands back, an existing key there must be
// read, never replaced by a freshly generated one. Regenerating would make every
// existing backup permanently undecryptable.
//
// The routing to a pre-existing key in the OS default directory is covered by
// the resolver's own tests (internal/conf/configdir_test.go); the real default
// directory cannot be redirected here because GetUserHomeDir is process-cached.
func TestEncryptionKeyNeverRegeneratedOverExistingKey(t *testing.T) {
	scratch := useConfigFlag(t)

	want := make([]byte, AES256KeySize)
	for i := range want {
		want[i] = byte(i)
	}
	keyFile := filepath.Join(scratch, EncryptionKeyFileName)
	require.NoError(t, os.WriteFile(keyFile, []byte(hex.EncodeToString(want)), PermSecureFile))

	m := &Manager{config: &conf.BackupConfig{Encryption: true}, logger: GetLogger()}
	got, err := m.getEncryptionKey()
	require.NoError(t, err)
	assert.Equal(t, want, got, "an existing key must be read, not regenerated")

	// And the file on disk is untouched.
	onDisk, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(want), string(onDisk))
}

// TestStateFileFollowsConfigFlag covers the backup state file.
func TestStateFileFollowsConfigFlag(t *testing.T) {
	scratch := useConfigFlag(t)

	sm, err := NewStateManager(GetLogger())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(scratch, BackupStateFileName), sm.statePath,
		"backup-state.json must be co-located with the --config file")

	// A save must land in the active location.
	require.NoError(t, sm.saveState())
	assert.FileExists(t, filepath.Join(scratch, BackupStateFileName))
}

// TestStateFileReadsLegacyLocation covers read-and-migrate for backup state: a
// pre-fix state file is still read, so schedules and statistics survive.
func TestStateFileReadsLegacyLocation(t *testing.T) {
	activeDir, legacyDir := t.TempDir(), t.TempDir()
	legacyState := filepath.Join(legacyDir, BackupStateFileName)
	require.NoError(t, os.WriteFile(legacyState,
		[]byte(`{"schedules":{"daily":{"failure_count":7}},"targets":{},"stats":{}}`), PermSecureFile))

	sm := &StateManager{
		statePath:       filepath.Join(activeDir, BackupStateFileName),
		legacyStatePath: legacyState,
		state:           &BackupState{},
		logger:          GetLogger(),
	}

	require.NoError(t, sm.loadState())
	require.Contains(t, sm.state.Schedules, "daily",
		"pre-existing backup state must survive the move")
	assert.Equal(t, 7, sm.state.Schedules["daily"].FailureCount)

	// The migration completes on the first save: it writes to the active path and
	// leaves the legacy file alone.
	legacyBefore, err := os.ReadFile(legacyState)
	require.NoError(t, err)
	require.NoError(t, sm.saveState())
	assert.FileExists(t, sm.statePath)
	legacyAfter, err := os.ReadFile(legacyState)
	require.NoError(t, err)
	assert.Equal(t, legacyBefore, legacyAfter, "the legacy state file must not be written to")
}
