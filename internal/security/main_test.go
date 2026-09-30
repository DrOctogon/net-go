package security

import (
	"fmt"
	"os"
	"testing"

	"github.com/tphakala/voicewatch/internal/conf/conftest"
)

func TestMain(m *testing.M) {
	conftest.NewTestSettings().Apply()

	// Sandbox the whole package away from the real ~/.config/birdnet-go.
	// NewOAuth2Server resolves both the session store and the persisted token
	// file through configBaseDir, so without this any test that constructs one
	// loads the developer's real tokens.json and writes test tokens back into
	// it. t.TempDir is unavailable in TestMain, hence the manual temp dir.
	//nolint:gocritic // ruleguard suggests t.TempDir/t.ArtifactDir; neither exists in TestMain, which has no *testing.T
	tmpDir, err := os.MkdirTemp("", "voicewatch-security-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp config dir: %v\n", err)
		os.Exit(1)
	}
	SetTestConfigPath(tmpDir)

	code := m.Run()

	SetTestConfigPath("")
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}
