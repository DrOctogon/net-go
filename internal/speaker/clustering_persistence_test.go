package speaker

import (
	"encoding/json"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/errors"
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

func TestClusterer_SaveFailsOnUnmarshalableThreshold(t *testing.T) {
	t.Parallel()

	// json.Marshal rejects NaN/Inf floats. NewClusterer now validates its
	// threshold (falling back to DefaultClusterThreshold for NaN/Inf/<=0/>1.0),
	// so this invalid state is no longer reachable through the constructor.
	// Construct it directly (same package) to keep Save's marshal-failure
	// branch covered.
	c := &Clusterer{threshold: math.NaN()}
	path := filepath.Join(t.TempDir(), "clusters.json")

	err := c.Save(path)
	require.Error(t, err)
	assert.True(t, errors.IsCategory(err, errors.CategoryFileIO))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "destination file must not be created on marshal failure")
}

func TestNewClustererFromSnapshot_ThresholdValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		threshold float64
		want      float64
	}{
		{"NewClustererFromSnapshot: zero threshold falls back to default", 0, DefaultClusterThreshold},
		{"NewClustererFromSnapshot: negative threshold falls back to default", -1, DefaultClusterThreshold},
		{"NewClustererFromSnapshot: NaN threshold falls back to default", math.NaN(), DefaultClusterThreshold},
		{"NewClustererFromSnapshot: infinite threshold falls back to default", math.Inf(1), DefaultClusterThreshold},
		{"NewClustererFromSnapshot: threshold above cosine max falls back to default", 1.5, DefaultClusterThreshold},
		{"NewClustererFromSnapshot: valid threshold is kept unchanged", 0.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := NewClustererFromSnapshot(SpeakerClusterSnapshot{Threshold: tt.threshold})
			assert.InDelta(t, tt.want, c.threshold, 1e-9)
		})
	}
}

func TestLoad_ThresholdValidation(t *testing.T) {
	t.Parallel()

	// NaN/Inf cannot round-trip through JSON, but out-of-range finite values
	// (and the historical <=0 case) can, so drive these through a real
	// Save-file's worth of JSON via Load rather than in-memory only.
	tests := []struct {
		name      string
		threshold float64
		want      float64
	}{
		{"Load: zero threshold falls back to default", 0, DefaultClusterThreshold},
		{"Load: negative threshold falls back to default", -1, DefaultClusterThreshold},
		{"Load: threshold above cosine max falls back to default", 1.5, DefaultClusterThreshold},
		{"Load: valid threshold is kept unchanged", 0.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "clusters.json")
			data, err := json.Marshal(SpeakerClusterSnapshot{Threshold: tt.threshold})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))

			loaded, err := Load(path)
			require.NoError(t, err)
			assert.InDelta(t, tt.want, loaded.threshold, 1e-9)
		})
	}
}

func TestNewClustererFromSnapshot_DropsMismatchedDimensionClusters(t *testing.T) {
	t.Parallel()

	// spk_2's centroid has a different dimension than the first valid
	// cluster (spk_1); it is permanently unmatchable (Cosine returns 0 on
	// length mismatch) and must be dropped rather than waste a slot.
	snap := SpeakerClusterSnapshot{
		Threshold: 0.75,
		NextID:    3,
		Clusters: []ClusterSnapshot{
			{ID: "spk_1", Centroid: []float32{1, 0, 0}, Count: 1},
			{ID: "spk_2", Centroid: []float32{0, 1}, Count: 1},
			{ID: "spk_3", Centroid: []float32{0, 1, 0}, Count: 1},
		},
	}

	c := NewClustererFromSnapshot(snap)
	require.Equal(t, 2, c.NumClusters())

	ids := make([]string, len(c.clusters))
	for i, cl := range c.clusters {
		ids[i] = cl.id
	}
	assert.ElementsMatch(t, []string{"spk_1", "spk_3"}, ids)
}

func TestNewClustererFromSnapshot_FirstValidClusterDimensionWins(t *testing.T) {
	t.Parallel()

	// The empty-centroid cluster is dropped before dimension is established, so
	// spk_12 (3-dim) sets the dimension, not spk_11's empty entry; spk_13
	// (2-dim) then mismatches and is dropped too.
	snap := SpeakerClusterSnapshot{
		Clusters: []ClusterSnapshot{
			{ID: "spk_11", Centroid: nil, Count: 1},
			{ID: "spk_12", Centroid: []float32{1, 0, 0}, Count: 1},
			{ID: "spk_13", Centroid: []float32{0, 1}, Count: 1},
		},
	}

	c := NewClustererFromSnapshot(snap)
	require.Equal(t, 1, c.NumClusters())
	assert.Equal(t, "spk_12", c.clusters[0].id)
}

func TestNewClustererFromSnapshot_NextIDMonotonic(t *testing.T) {
	t.Parallel()

	t.Run("nextID rises above the max restored spk_N suffix", func(t *testing.T) {
		t.Parallel()
		snap := SpeakerClusterSnapshot{
			Threshold: 0.75,
			NextID:    1, // lower than the existing spk_5
			Clusters: []ClusterSnapshot{
				{ID: "spk_5", Centroid: []float32{1, 0, 0}, Count: 1},
			},
		}
		c := NewClustererFromSnapshot(snap)

		// A brand-new orthogonal voice must not collide with spk_1..spk_5.
		id, isNew := c.AssignWithNovelty([]float32{0, 1, 0})
		assert.True(t, isNew)
		assert.Equal(t, "spk_6", id)
	})

	t.Run("non-conforming IDs are ignored when computing the max suffix", func(t *testing.T) {
		t.Parallel()
		snap := SpeakerClusterSnapshot{
			NextID: 1,
			Clusters: []ClusterSnapshot{
				{ID: "custom-id", Centroid: []float32{1, 0, 0}, Count: 1},
				{ID: "spk_abc", Centroid: []float32{0, 1, 0}, Count: 1},
			},
		}
		c := NewClustererFromSnapshot(snap)

		id, isNew := c.AssignWithNovelty([]float32{0, 0, 1})
		assert.True(t, isNew)
		assert.Equal(t, "spk_2", id, "malformed IDs must not raise nextID")
	})

	t.Run("snapshot NextID higher than any restored suffix is kept", func(t *testing.T) {
		t.Parallel()
		snap := SpeakerClusterSnapshot{
			NextID: 9,
			Clusters: []ClusterSnapshot{
				{ID: "spk_2", Centroid: []float32{1, 0, 0}, Count: 1},
			},
		}
		c := NewClustererFromSnapshot(snap)

		id, isNew := c.AssignWithNovelty([]float32{0, 1, 0})
		assert.True(t, isNew)
		assert.Equal(t, "spk_10", id)
	})
}

func TestClusterer_SnapshotRestoreRoundTripSanity(t *testing.T) {
	t.Parallel()

	// Combine all three hardenings: an invalid threshold, a mismatched-dimension
	// cluster, and a stale NextID lower than an existing spk_N suffix.
	snap := SpeakerClusterSnapshot{
		Threshold: -1,
		NextID:    1,
		Clusters: []ClusterSnapshot{
			{ID: "spk_7", Centroid: []float32{1, 0, 0}, Count: 3},
			{ID: "spk_8", Centroid: []float32{0, 1}, Count: 1}, // wrong dim
		},
	}

	c := NewClustererFromSnapshot(snap)
	assert.InDelta(t, DefaultClusterThreshold, c.threshold, 1e-9)
	require.Equal(t, 1, c.NumClusters())

	// The surviving cluster still matches its original voice.
	assert.Equal(t, "spk_7", c.Assign([]float32{1, 0, 0}))
	// A new voice gets an ID past the highest restored suffix, not spk_2.
	id, isNew := c.AssignWithNovelty([]float32{0, 1, 0})
	assert.True(t, isNew)
	assert.Equal(t, "spk_8", id)
}

func TestClusterer_SaveFailsOnReadOnlyDir(t *testing.T) {
	t.Parallel()

	// chmod cannot make a directory unwritable for its owner on Windows, so
	// os.CreateTemp would still succeed there.
	if runtime.GOOS == "windows" {
		t.Skip("chmod cannot make a directory unwritable for the owner on Windows")
	}

	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	c := NewClusterer(0)
	c.Assign(oneHot(4, 0))
	path := filepath.Join(dir, "clusters.json")

	err := c.Save(path)
	require.Error(t, err)
	assert.True(t, errors.IsCategory(err, errors.CategoryFileIO))
}

func TestClusterer_SaveFailsWhenDestIsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")
	require.NoError(t, os.Mkdir(path, 0o755))

	c := NewClusterer(0)
	c.Assign(oneHot(4, 0))

	err := c.Save(path)
	require.Error(t, err)
	assert.True(t, errors.IsCategory(err, errors.CategoryFileIO))

	// os.Rename must have failed cleanly: the temp file is cleaned up and the
	// directory at path is left untouched.
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Len(t, entries, 1, "failed rename must not leave a stray temp file behind")
	assert.Equal(t, "clusters.json", entries[0].Name())
	assert.True(t, entries[0].IsDir())
}
