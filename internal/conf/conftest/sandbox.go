package conftest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/conf"
)

// sandboxConfigSeed is the placeholder content written to a sandboxed config
// file. conf.FindConfigFile only needs the file to exist; anything that saves
// settings overwrites it wholesale.
const sandboxConfigSeed = "# sandboxed test config - written by conftest.SandboxConfigFile\n"

// SandboxConfigFile points configuration resolution at a throwaway config.yaml
// inside tb.TempDir() and restores the previous conf.ConfigPath on cleanup. It
// returns the path to the sandboxed file.
//
// Use it in any test that persists configuration - conf.SaveSettings,
// conf.SaveYAMLConfig, or a handler that reaches them. Without it those calls
// resolve to the machine owner's real ~/.config/birdnet-go/config.yaml, and the
// guard in internal/conf/config_guard.go fails the write rather than letting a
// test rewrite a developer's configuration.
//
// It also keeps conf.FindConfigFile and conf.ResolveConfigDir pointed at the
// sandbox. Those reads are not guarded, so without this helper they silently
// resolve to the developer's real config instead.
//
// IMPORTANT: tests using this helper must NOT call t.Parallel(). conf.ConfigPath
// is a process-global variable, so parallel tests would observe each other's
// sandbox paths and flake.
//
// Pair it with SetTestSettings when the test also needs a published snapshot:
//
//	conftest.SetTestSettings(conftest.NewTestSettings().Build())
//	conftest.SandboxConfigFile(t)
//	require.NoError(t, conf.SaveSettings())
func SandboxConfigFile(tb testing.TB) string {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "config.yaml")
	require.NoError(tb, os.WriteFile(path, []byte(sandboxConfigSeed), 0o600),
		"conftest: failed to seed the sandboxed config file")

	prev := conf.ConfigPath
	conf.ConfigPath = path
	tb.Cleanup(func() { conf.ConfigPath = prev })

	return path
}
