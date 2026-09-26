// persist_status.go: process-wide record of the most recent failure to
// persist generated configuration back to disk. Startup migrations (e.g. the
// session-secret backfill) deliberately do not fail startup when the config
// file is unwritable, but the consequence — a session secret that regenerates
// on every restart, silently invalidating all sessions — should be visible on
// the health page rather than only in logs.
package conf

import (
	"sync/atomic"
	"time"
)

// ConfigPersistFailure describes a failed attempt to write generated
// configuration back to the config file.
type ConfigPersistFailure struct {
	Operation string    // what was being persisted, e.g. "session_secret"
	Path      string    // config file path
	Error     string    // underlying error text
	At        time.Time // when the failure happened
}

var lastConfigPersistFailure atomic.Pointer[ConfigPersistFailure]

// recordConfigPersistFailure notes a failed config write for health reporting.
func recordConfigPersistFailure(operation, path string, err error) {
	lastConfigPersistFailure.Store(&ConfigPersistFailure{
		Operation: operation,
		Path:      path,
		Error:     err.Error(),
		At:        time.Now(),
	})
}

// LastConfigPersistFailure returns the most recent config-persistence failure,
// or nil when none has occurred since process start.
func LastConfigPersistFailure() *ConfigPersistFailure {
	return lastConfigPersistFailure.Load()
}
