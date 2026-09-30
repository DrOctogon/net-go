package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
)

// useConfigFlag exercises the production resolver instead of the package test
// hook: it clears testConfigPath and points conf.ConfigPath at a config.yaml in
// a temp directory, restoring both afterwards. Everything resolved through it
// therefore lands under t.TempDir(), never in the real ~/.config/birdnet-go.
//
// Both variables are process-global, so tests using this must not be parallel.
func useConfigFlag(t *testing.T) string {
	t.Helper()

	scratch := t.TempDir()
	configFile := filepath.Join(scratch, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("debug: false\n"), 0o600))

	prevTestPath, prevConfigPath := testConfigPath, conf.ConfigPath
	t.Cleanup(func() {
		SetTestConfigPath(prevTestPath)
		conf.ConfigPath = prevConfigPath
	})
	SetTestConfigPath("")
	conf.ConfigPath = configFile

	return scratch
}

// TestTokenFileFollowsConfigFlag is the regression test for the reported bug: a
// server started with --config must persist tokens beside that config file, not
// in the OS default config directory.
func TestTokenFileFollowsConfigFlag(t *testing.T) {
	scratch := useConfigFlag(t)

	server := &OAuth2Server{
		authCodes:    make(map[string]AuthCode),
		accessTokens: make(map[string]AccessToken),
	}
	server.setupTokenPersistence()

	require.True(t, server.persistTokens, "persistence must stay enabled")
	assert.Equal(t, filepath.Join(scratch, TokensFileName), server.tokensFile,
		"tokens.json must be co-located with the --config file")

	// legacyTokensFile depends on whether the machine running the test happens to
	// have a pre-fix tokens.json in its default config directory, so its value is
	// not asserted here (the fallback has its own tests). What must hold either
	// way is that it is only ever a *different*, read-only path: writes go to
	// tokensFile, which is inside the --config directory.
	if server.legacyTokensFile != "" {
		assert.NotEqual(t, server.tokensFile, server.legacyTokensFile)
		assert.NotContains(t, server.legacyTokensFile, scratch)
	}
}

// TestSessionPathFollowsConfigFlag is the same check for the session store.
func TestSessionPathFollowsConfigFlag(t *testing.T) {
	scratch := useConfigFlag(t)

	sessionPath, ok := getSessionPath()
	require.True(t, ok)
	assert.Equal(t, filepath.Join(scratch, SessionsDirName), sessionPath,
		"the session store must be co-located with the --config file")
}

// TestReadTokensFileFallsBackToLegacyLocation covers the migration hazard for
// tokens: a token file written by a pre-fix release must still be read, so
// upgrading does not log everyone out.
func TestReadTokensFileFallsBackToLegacyLocation(t *testing.T) {
	activeDir, legacyDir := t.TempDir(), t.TempDir()
	legacyTokens := filepath.Join(legacyDir, TokensFileName)
	writeTokenFile(t, legacyTokens, "legacy-token")

	server := &OAuth2Server{
		authCodes:        make(map[string]AuthCode),
		accessTokens:     make(map[string]AccessToken),
		tokensFile:       filepath.Join(activeDir, TokensFileName),
		legacyTokensFile: legacyTokens,
		persistTokens:    true,
	}

	require.NoError(t, server.loadTokens(t.Context()))
	assert.NoError(t, server.ValidateAccessToken("legacy-token"),
		"a pre-existing token must survive the move to the new location")
}

// TestReadTokensFilePrefersActiveLocation verifies the fallback stops applying
// once the active location has a token file: the migration completes on the
// first save and is never undone.
func TestReadTokensFilePrefersActiveLocation(t *testing.T) {
	activeDir, legacyDir := t.TempDir(), t.TempDir()
	activeTokens := filepath.Join(activeDir, TokensFileName)
	writeTokenFile(t, activeTokens, "active-token")
	legacyTokens := filepath.Join(legacyDir, TokensFileName)
	writeTokenFile(t, legacyTokens, "legacy-token")

	server := &OAuth2Server{
		authCodes:        make(map[string]AuthCode),
		accessTokens:     make(map[string]AccessToken),
		tokensFile:       activeTokens,
		legacyTokensFile: legacyTokens,
		persistTokens:    true,
	}

	require.NoError(t, server.loadTokens(t.Context()))
	assert.NoError(t, server.ValidateAccessToken("active-token"))
	assert.Error(t, server.ValidateAccessToken("legacy-token"),
		"the legacy file must be ignored once the active one exists")
}

// TestSaveTokensWritesToActiveLocationOnly pins the read-and-migrate contract:
// reads may come from the legacy file, but writes never go back to it.
func TestSaveTokensWritesToActiveLocationOnly(t *testing.T) {
	activeDir, legacyDir := t.TempDir(), t.TempDir()
	legacyTokens := filepath.Join(legacyDir, TokensFileName)
	writeTokenFile(t, legacyTokens, "legacy-token")
	legacyBefore, err := os.ReadFile(legacyTokens)
	require.NoError(t, err)

	activeTokens := filepath.Join(activeDir, TokensFileName)
	server := &OAuth2Server{
		authCodes:        make(map[string]AuthCode),
		accessTokens:     map[string]AccessToken{"fresh": {Token: "fresh", ExpiresAt: time.Now().Add(time.Hour)}},
		tokensFile:       activeTokens,
		legacyTokensFile: legacyTokens,
		persistTokens:    true,
	}

	require.NoError(t, server.saveTokens(t.Context()))

	assert.FileExists(t, activeTokens, "the save must land in the active location")
	legacyAfter, err := os.ReadFile(legacyTokens)
	require.NoError(t, err)
	assert.Equal(t, legacyBefore, legacyAfter, "the legacy token file must not be written to")
}

func writeTokenFile(t *testing.T, path, token string) {
	t.Helper()
	payload := struct {
		AuthCodes    map[string]AuthCode    `json:"auth_codes"`
		AccessTokens map[string]AccessToken `json:"access_tokens"`
	}{
		AuthCodes: map[string]AuthCode{},
		AccessTokens: map[string]AccessToken{
			token: {Token: token, ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, FilePermissions))
}

// TestConfigArtifactPathHonorsTestHook guards the PR #63 sandbox: with the test
// hook set, resolution must stay inside the hook directory and never consult the
// real config directory. A regression here corrupts the developer's tokens.json.
func TestConfigArtifactPathHonorsTestHook(t *testing.T) {
	hookDir := t.TempDir()
	prev := testConfigPath
	t.Cleanup(func() { SetTestConfigPath(prev) })
	SetTestConfigPath(hookDir)

	got, err := configArtifactPath(TokensFileName)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(hookDir, TokensFileName), got)

	assert.Empty(t, legacyConfigArtifactPath(TokensFileName),
		"the test hook must suppress the legacy fallback into the real config directory")
}
