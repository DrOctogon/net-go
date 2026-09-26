// backup_race_test.go: guards against the backupJobManager lazy-init race.

package api

import (
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackupJobManagerSingleton_ConcurrentInit exercises concurrent calls to
// initBackupRoutes. Before the sync.Once guard, backupJobManager was set via
// a bare "if backupJobManager == nil { backupJobManager = ... }" check: with
// several goroutines racing through it (e.g. parallel tests each creating a
// Controller with route init), more than one could observe nil and construct
// its own manager, and -race flags the unsynchronized read/write of the
// package-level pointer. This test fails under -race on the unfixed code and
// passes once the singleton init is serialized by sync.Once.
func TestBackupJobManagerSingleton_ConcurrentInit(t *testing.T) {
	t.Cleanup(func() {
		if backupJobManager != nil {
			backupJobManager.Shutdown()
		}
		backupJobManager = nil
		backupJobManagerOnce = sync.Once{}
	})

	const goroutines = 8

	var wg sync.WaitGroup
	managers := make([]*BackupJobManager, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := echo.New()
			c := newMinimalController()
			c.Group = e.Group(apiV2Prefix)
			c.initBackupRoutes()
			// Reading the package-level singleton here is exactly the access
			// pattern production handlers use; collect into a slice rather
			// than asserting from the goroutine (TESTING.md: no testify
			// calls from goroutines).
			managers[i] = backupJobManager
		}(i)
	}
	wg.Wait()

	require.NotNil(t, managers[0])
	for i := 1; i < goroutines; i++ {
		assert.Same(t, managers[0], managers[i], "all callers must observe the same singleton instance")
	}
}
