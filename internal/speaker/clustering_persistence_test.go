package speaker

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oneHot returns a length-n embedding that is 1 at index hot and 0 elsewhere.
// Distinct hot indices are orthogonal (cosine 0), so they always form separate
// speaker clusters — handy for deterministic clustering assertions.
func oneHot(n, hot int) []float32 {
	e := make([]float32, n)
	e[hot] = 1
	return e
}

func TestClusterer_SnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	id1 := c.Assign(oneHot(8, 0)) // spk_1
	id2 := c.Assign(oneHot(8, 4)) // spk_2 (orthogonal → new cluster)
	c.Assign(oneHot(8, 0))        // folds into spk_1
	require.Equal(t, "spk_1", id1)
	require.Equal(t, "spk_2", id2)
	require.Equal(t, 2, c.NumClusters())

	snap := c.Snapshot()
	assert.InDelta(t, 0.75, snap.Threshold, 1e-9)
	assert.Equal(t, 2, snap.NextID)
	require.Len(t, snap.Clusters, 2)

	restored := NewClustererFromSnapshot(snap)
	assert.Equal(t, 2, restored.NumClusters())

	// A near-duplicate of the first speaker must rejoin spk_1, not open spk_3.
	assert.Equal(t, "spk_1", restored.Assign(oneHot(8, 0)))
	// A brand-new orthogonal speaker must get the next fresh id after the restore.
	assert.Equal(t, "spk_3", restored.Assign(oneHot(8, 7)))
}

func TestClusterer_SaveLoadFile(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0)
	c.Assign(oneHot(4, 0))
	c.Assign(oneHot(4, 2))
	path := filepath.Join(t.TempDir(), "clusters.json")

	require.NoError(t, c.Save(path))

	loaded, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, c.NumClusters(), loaded.NumClusters())
	assert.Equal(t, "spk_1", loaded.Assign(oneHot(4, 0))) // rejoin, not a new id
}

func TestClusterer_LoadMissingFile(t *testing.T) {
	t.Parallel()

	loaded, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	assert.Nil(t, loaded)
	assert.ErrorIs(t, err, fs.ErrNotExist, "missing file must be distinguishable via fs.ErrNotExist")
}

func TestClusterer_SaveAtomicOverwrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "clusters.json")
	first := NewClusterer(0)
	first.Assign(oneHot(4, 0))
	require.NoError(t, first.Save(path))

	// Overwrite with a two-cluster clusterer; no stray temp files must remain.
	second := NewClusterer(0)
	second.Assign(oneHot(4, 0))
	second.Assign(oneHot(4, 2))
	require.NoError(t, second.Save(path))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 2, loaded.NumClusters())

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "atomic save must not leave temp files behind")
}

func TestClusterer_LoadRejectsCorruptJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "clusters.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	loaded, err := Load(path)
	assert.Nil(t, loaded)
	require.Error(t, err)
	assert.NotErrorIs(t, err, fs.ErrNotExist, "corrupt content is not a missing-file error")
}

func TestClusterer_SnapshotEmpty(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0)
	snap := c.Snapshot()
	assert.Empty(t, snap.Clusters)
	assert.Equal(t, 0, snap.NextID)

	restored := NewClustererFromSnapshot(snap)
	assert.Equal(t, 0, restored.NumClusters())
	assert.Equal(t, "spk_1", restored.Assign(oneHot(4, 0)))
}

func TestClusterer_SnapshotIsolation(t *testing.T) {
	t.Parallel()

	// Mutating the clusterer after snapshotting must not alter the snapshot, and
	// mutating the snapshot's slices must not corrupt the live clusterer.
	c := NewClusterer(0)
	c.Assign(oneHot(4, 0))
	snap := c.Snapshot()

	c.Assign(oneHot(4, 2)) // live clusterer grows to 2
	assert.Len(t, snap.Clusters, 1, "snapshot must be a deep copy, unaffected by later Assign")

	snap.Clusters[0].Centroid[0] = 999 // corrupt the snapshot copy
	assert.Equal(t, "spk_1", c.Assign(oneHot(4, 0)), "live centroid must be untouched by snapshot mutation")
}
