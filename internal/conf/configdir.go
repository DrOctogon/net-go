package conf

import (
	"os"
	"path/filepath"

	"github.com/tphakala/voicewatch/internal/errors"
)

// ConfigArtifactPath returns the path of a per-installation artifact that
// belongs beside this installation's config.yaml — the OAuth token file, the
// session store, the backup state file, the backup encryption key.
//
// The path is resolved from the *active* config file (so it honours the
// --config/-c flag via ResolveConfigDir) rather than from the OS default config
// directory. Resolving from the default directory is what let two instances
// started with different --config files silently share tokens, sessions, backup
// state and the backup encryption key, and what made those artifacts land
// outside a container's mounted /config and vanish on restart.
//
// This is the path artifacts are WRITTEN to. Callers that must keep data written
// before --config was honoured reachable pair it with LegacyConfigArtifact.
//
// For a default-path install the active config directory *is* the default
// directory, so nothing moves and behaviour is unchanged.
func ConfigArtifactPath(name string) (string, error) {
	configDir, err := ResolveConfigDir()
	if err != nil {
		return "", errors.New(err).
			Category(errors.CategoryConfiguration).
			Context("operation", "config-artifact-path").
			Context("artifact", name).
			Build()
	}
	return filepath.Join(configDir, name), nil
}

// LegacyConfigArtifact returns the pre-fix location of the named artifact — the
// OS default config directory, which is where every caller resolved its paths
// from before --config was honoured — but only when there is something there
// worth reading and the active location has nothing.
//
// It returns "" when there is no migration to perform: the active location
// already holds the artifact (the active copy always wins), the default
// directory is the active directory, it cannot be resolved, or the artifact is
// simply not there. Callers treat a non-empty result as a read-only fallback.
func LegacyConfigArtifact(name string) string {
	activePath, err := ConfigArtifactPath(name)
	if err != nil {
		return ""
	}

	defaultPaths, err := GetDefaultConfigPaths()
	if err != nil || len(defaultPaths) == 0 {
		return ""
	}
	return legacyArtifactFallback(name, activePath, defaultPaths[0])
}

// legacyArtifactFallback holds the fallback decision, separated from resolving
// the OS default directory so it can be tested against temporary directories
// instead of the real config directory of whoever runs the tests.
func legacyArtifactFallback(name, activePath, legacyDir string) string {
	// The active copy always wins: once an artifact has been written to the
	// active location the migration is complete and must never be undone.
	if configArtifactExists(activePath) {
		return ""
	}

	legacyPath := filepath.Join(legacyDir, name)
	if legacyPath == activePath || !configArtifactExists(legacyPath) {
		return ""
	}
	return legacyPath
}

// ResolveConfigArtifact returns the single path to use for both reading and
// writing the named artifact: the active location, or a pre-existing artifact in
// the OS default directory when the active location has none.
//
// This is the "read-in-place" variant, for artifacts that must never become
// unfindable and whose readers and writers must agree on one path — the backup
// encryption key above all, where a key the process cannot find is not an
// inconvenience but permanently undecryptable backups. It deliberately leaves
// such an install pinned to the legacy file; that is the correct trade for a
// secret, but it does mean the install gains no isolation for that artifact
// until the legacy file is removed. Artifacts that can be migrated safely should
// write to ConfigArtifactPath and fall back to LegacyConfigArtifact for reads
// instead, which completes the migration on the first write.
func ResolveConfigArtifact(name string) (string, error) {
	if legacy := LegacyConfigArtifact(name); legacy != "" {
		return legacy, nil
	}
	return ConfigArtifactPath(name)
}

// configArtifactExists reports whether a config artifact is actually present at
// path. A directory counts only when it holds at least one entry: the session
// store directory is routinely created empty (a failed store init falls back to
// cookies), and an empty directory is not data worth pinning an install to.
func configArtifactExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return true
	}

	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}
