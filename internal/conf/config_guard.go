// conf/config_guard.go protects the machine owner's real configuration file
// from being overwritten by the test suite.
//
// Background: conf.SaveSettings resolves its destination through
// FindConfigFile, which falls back to GetDefaultConfigPaths() and therefore to
// the developer's own $HOME/.config/birdnet-go/config.yaml. A test that
// publishes settings and then triggers a save (directly, or indirectly through
// an HTTP handler) silently rewrites that file with test values. This has
// happened: a model-manager test wrote temp-dir model paths into a developer's
// real config.
//
// Redirecting HOME is not a usable defence because GetUserHomeDir prefers
// os/user.Current(), which reads the password database and ignores $HOME. The
// guard below therefore compares against the same directories the production
// resolver would use, and - only when running under `go test` - refuses the
// operation with an actionable error instead of touching the file.
//
// Scope: SaveYAMLConfig only. That is the single chokepoint for destructive
// configuration writes - SaveSettings and both migrations route through it -
// and it is the shape the historical incident took.
//
// Two other call sites were guarded in the first revision of this file and are
// deliberately left unguarded now, because guarding reads has a far wider blast
// radius than the harm it prevents:
//
//   - FindConfigFile (resolution). A test that merely reads the developer's
//     config is untidy, not destructive.
//   - createDefaultConfig (provisioning). Load() reaches it whenever viper
//     finds no config file, so on a CI runner with no config it is on the path
//     of every test that lazily calls conf.Setting(). Guarding it made Load()
//     fail, conf.Setting() return nil, and unrelated packages crash on the nil
//     snapshot. It also cannot clobber a developer's settings: it only runs
//     when no config file exists yet.
package conf

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tphakala/voicewatch/internal/errors"
)

const (
	// appConfigDirName is the per-application directory name used inside the
	// user's configuration directory and under /etc.
	appConfigDirName = "birdnet-go"

	// systemConfigDir is the system-wide configuration directory on unix hosts.
	systemConfigDir = "/etc/" + appConfigDirName

	// configFileName is the base name of the YAML configuration file.
	configFileName = "config.yaml"

	// testSandboxHint tells the developer how to opt a test into a sandboxed
	// configuration file instead of the machine owner's real one.
	testSandboxHint = "tests must not read or write the machine's real configuration; " +
		"call conftest.SandboxConfigFile(t) to point config resolution at a throwaway " +
		"file under t.TempDir(), or set conf.ConfigPath to a temp path yourself"
)

// userConfigDir returns the per-user configuration directory for the current
// operating system. It is the directory that holds config.yaml for a normal
// installation, and the directory the test guard protects.
func userConfigDir() (string, error) {
	homeDir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == osWindows {
		return filepath.Join(homeDir, "AppData", "Roaming", appConfigDirName), nil
	}
	return filepath.Join(homeDir, ".config", appConfigDirName), nil
}

// realConfigDirs returns the configuration directories that belong to the host
// machine rather than to a test. A missing home directory yields the
// system-wide directory only: if home cannot be resolved then the production
// resolver cannot produce a home-based path either, so there is nothing to
// guard.
func realConfigDirs() []string {
	dirs := make([]string, 0, 2)
	dirs = append(dirs, systemConfigDir)
	if dir, err := userConfigDir(); err == nil {
		dirs = append(dirs, dir)
	}
	return dirs
}

// pathWithin reports whether path is dir itself or lives underneath it. Both
// sides are made absolute first so that a volume-relative literal such as
// "/etc/birdnet-go" still compares correctly on Windows.
func pathWithin(dir, path string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// guardRealConfigPath returns a non-nil error when the test suite is about to
// overwrite the machine owner's real configuration file.
//
// Outside `go test` it always returns nil, so production saving is unchanged
// and a user can never trip it. Under `go test` it fails loudly rather than
// silently redirecting, because a silent redirect would hide the bug it exists
// to surface.
func guardRealConfigPath(path string) error {
	if !testing.Testing() {
		return nil
	}
	for _, dir := range realConfigDirs() {
		if !pathWithin(dir, path) {
			continue
		}
		return errors.Newf("refusing to use the real user config path %q from a test: %s", path, testSandboxHint).
			Category(errors.CategoryConfiguration).
			Context("operation", "guard-real-config-path").
			Context("path", path).
			Context("guarded_dir", dir).
			Build()
	}
	return nil
}
