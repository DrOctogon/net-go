// speaker_cluster_autosave_test.go: tests for the periodic voice-print cluster
// autosave. Timing is driven by testing/synctest, so the ticker fires on a
// virtual clock instead of a real sleep.
package processor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// autosaveSentinel is written over the snapshot file so a test can prove a
// skipped tick left the bytes untouched.
const autosaveSentinel = "sentinel-not-a-snapshot"

// pastTick advances the virtual clock just past one autosave interval and waits
// for the autosave goroutine to finish reacting to the tick.
func pastTick(t *testing.T) {
	t.Helper()
	time.Sleep(speakerClusterAutosaveInterval + time.Second)
	synctest.Wait()
}

func TestSpeakerClusterAutosave_WritesChangesAndSkipsUnchanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, speakerClusterStateFile)

		p := &Processor{Settings: sqliteSettings(dir)}
		p.speakerClusterer = speaker.NewClusterer(0)
		require.Equal(t, opsTargetID, p.speakerClusterer.Assign(speakerOneHot(8, 0)))

		ctx, cancel := context.WithCancel(t.Context())
		p.startSpeakerClusterAutosave(ctx)

		// A tick with unsaved clusters writes the snapshot.
		pastTick(t)
		require.FileExists(t, path, "first tick must persist the learned cluster")
		loaded, err := speaker.Load(path)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded.NumClusters())

		// A tick with nothing changed must not write at all: a Pi rewriting an
		// identical snapshot to its SD card every interval is pure wear.
		require.NoError(t, os.WriteFile(path, []byte(autosaveSentinel), 0o600))
		pastTick(t)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, autosaveSentinel, string(data), "an unchanged tick must not write")

		// A new speaker makes it dirty, so the next tick writes again.
		require.Equal(t, opsSourceID, p.speakerClusterer.Assign(speakerOneHot(8, 4)))
		pastTick(t)
		loaded, err = speaker.Load(path)
		require.NoError(t, err)
		assert.Equal(t, 2, loaded.NumClusters(), "a changed tick must rewrite the snapshot")

		// Cancelling must stop the ticker and end the goroutine; synctest.Test
		// fails the test if it is still alive when the bubble ends, and the
		// package-level goleak check catches a leak across the whole run.
		cancel()
		synctest.Wait()
	})
}

func TestSpeakerClusterAutosave_WriteErrorIsSurvivedAndRetried(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, speakerClusterStateFile)
		// A directory where the snapshot belongs makes Save's final rename fail.
		require.NoError(t, os.Mkdir(path, 0o755))

		p := &Processor{Settings: sqliteSettings(dir)}
		p.speakerClusterer = speaker.NewClusterer(0)
		p.speakerClusterer.Assign(speakerOneHot(8, 0))

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		p.startSpeakerClusterAutosave(ctx)

		// Several failing ticks must not panic or kill the goroutine.
		assert.NotPanics(t, func() {
			pastTick(t)
			pastTick(t)
		})

		// Once the obstruction is gone the next tick retries — the cluster was
		// never dropped from memory.
		require.NoError(t, os.Remove(path))
		pastTick(t)
		loaded, err := speaker.Load(path)
		require.NoError(t, err)
		assert.Equal(t, 1, loaded.NumClusters(), "the tick after a failure must retry the save")

		cancel()
		synctest.Wait()
	})
}

func TestSpeakerClusterAutosave_NoClustererStartsNoGoroutine(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Voice-print clustering disabled → no clusterer → nothing to autosave.
		p := &Processor{Settings: sqliteSettings(t.TempDir())}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		p.startSpeakerClusterAutosave(ctx)

		pastTick(t)
	})
}

func TestSpeakerClusterAutosave_SkipsTickWithoutStatePath(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()

		// SQLite disabled → no stable state directory → no snapshot to write.
		p := &Processor{Settings: &conf.Settings{}}
		p.speakerClusterer = speaker.NewClusterer(0)
		p.speakerClusterer.Assign(speakerOneHot(8, 0))

		ctx, cancel := context.WithCancel(t.Context())
		p.startSpeakerClusterAutosave(ctx)

		// The goroutine still runs (the state path is resolved per tick so a
		// settings change is picked up without a restart) but writes nothing.
		pastTick(t)
		assert.NoFileExists(t, filepath.Join(dir, speakerClusterStateFile))

		cancel()
		synctest.Wait()
	})
}
