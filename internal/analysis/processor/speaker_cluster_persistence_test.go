package processor

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// sqliteSettings returns settings whose SQLite store points at dir/db.sqlite so
// the speaker-cluster snapshot lands in dir.
func sqliteSettings(dir string) *conf.Settings {
	s := &conf.Settings{}
	s.Output.SQLite.Enabled = true
	s.Output.SQLite.Path = filepath.Join(dir, "db.sqlite")
	return s
}

// speakerOneHot returns a length-n embedding that is 1 at index hot; distinct
// indices are orthogonal, so they always form separate speaker clusters.
func speakerOneHot(n, hot int) []float32 {
	e := make([]float32, n)
	e[hot] = 1
	return e
}

func TestSpeakerClusterStatePath_SkippedWithoutSQLite(t *testing.T) {
	t.Parallel()

	p := &Processor{Settings: &conf.Settings{}} // SQLite disabled by default
	_, ok := p.speakerClusterStatePath()
	assert.False(t, ok, "persistence must be skipped when SQLite is not the store")
}

func TestSpeakerClusterStatePath_BesideDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := &Processor{Settings: sqliteSettings(dir)}
	path, ok := p.speakerClusterStatePath()
	require.True(t, ok)
	assert.Equal(t, filepath.Join(dir, speakerClusterStateFile), path)
}

func TestSpeakerClusters_PersistRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	settings := sqliteSettings(dir)

	// First process: two distinct speakers, then persist on shutdown.
	p1 := &Processor{Settings: settings}
	p1.speakerClusterer = speaker.NewClusterer(0)
	require.Equal(t, "spk_1", p1.speakerClusterer.Assign(speakerOneHot(8, 0)))
	require.Equal(t, "spk_2", p1.speakerClusterer.Assign(speakerOneHot(8, 4)))
	p1.persistSpeakerClusters()

	// Second process: restore, then a returning speaker must keep its old ID and
	// a new speaker must continue the counter (spk_3), not collide.
	p2 := &Processor{Settings: settings}
	p2.speakerClusterer = speaker.NewClusterer(0)
	p2.restoreSpeakerClusters()
	assert.Equal(t, 2, p2.speakerClusterer.NumClusters())
	assert.Equal(t, "spk_1", p2.speakerClusterer.Assign(speakerOneHot(8, 0)))
	assert.Equal(t, "spk_3", p2.speakerClusterer.Assign(speakerOneHot(8, 7)))
}

func TestSpeakerClusters_RestoreFirstRunIsNoop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := &Processor{Settings: sqliteSettings(dir)}
	fresh := speaker.NewClusterer(0)
	p.speakerClusterer = fresh

	p.restoreSpeakerClusters() // no snapshot yet → must keep the fresh clusterer

	assert.Same(t, fresh, p.speakerClusterer, "first run must leave the fresh clusterer untouched")
	assert.Equal(t, "spk_1", p.speakerClusterer.Assign(speakerOneHot(4, 0)))
}

func TestSpeakerClusters_PersistNoopWhenDisabled(t *testing.T) {
	t.Parallel()

	// No SQLite store → no state path → persist is a silent no-op (must not panic).
	p := &Processor{Settings: &conf.Settings{}}
	p.speakerClusterer = speaker.NewClusterer(0)
	p.speakerClusterer.Assign(speakerOneHot(4, 0))
	assert.NotPanics(t, p.persistSpeakerClusters)
}
