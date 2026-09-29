// clustering_ops_test.go: tests for operator corrections to the greedy online
// clusterer — Merge (one person split across two ids) and Remove (forget a
// voice) — plus their interaction with snapshot/restore.
package speaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cluster ids minted by the first assignments in these tests.
const (
	opsID1 = "spk_1"
	opsID2 = "spk_2"
	opsID3 = "spk_3"
	// opsIDAbsent and opsIDAbsent2 are ids no test ever mints.
	opsIDAbsent  = "spk_9"
	opsIDAbsent2 = "spk_8"
)

// centroidOf returns a copy of the named cluster's centroid and member count,
// failing the test when no such cluster exists.
func centroidOf(t *testing.T, c *Clusterer, id string) (centroid []float32, count int) {
	t.Helper()
	snap := c.Snapshot()
	for i := range snap.Clusters {
		if snap.Clusters[i].ID == id {
			return snap.Clusters[i].Centroid, snap.Clusters[i].Count
		}
	}
	t.Fatalf("cluster %s not found", id)
	return nil, 0
}

func TestClusterer_Merge_WeightedCentroidAndCounts(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	// spk_1 gets three members at [1,0,0,0]; spk_2 gets one at [0,1,0,0].
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))
	require.Equal(t, 2, c.NumClusters())

	require.NoError(t, c.Merge(opsID1, opsID2))

	// Source gone, target survives, nothing else created.
	assert.Equal(t, 1, c.NumClusters())
	assert.True(t, c.HasCluster(opsID1))
	assert.False(t, c.HasCluster(opsID2))

	centroid, count := centroidOf(t, c, opsID1)
	assert.Equal(t, 4, count, "counts must sum")
	// (3*[1,0,0,0] + 1*[0,1,0,0]) / 4 = [0.75, 0.25, 0, 0].
	require.Len(t, centroid, 4)
	assert.InDelta(t, 0.75, centroid[0], 1e-6)
	assert.InDelta(t, 0.25, centroid[1], 1e-6)
	assert.InDelta(t, 0.0, centroid[2], 1e-6)
	assert.InDelta(t, 0.0, centroid[3], 1e-6)
}

func TestClusterer_Merge_TargetMatchesBothFormerMembers(t *testing.T) {
	t.Parallel()

	// Threshold low enough that the merged centroid, which sits midway between
	// the two former centroids, is still similar enough to either side.
	const threshold = 0.5
	c := NewClusterer(threshold)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))

	require.NoError(t, c.Merge(opsID1, opsID2))

	// Assert against the merged centroid directly: a live Assign would fold the
	// embedding in and move the centroid before the second assertion runs.
	centroid, _ := centroidOf(t, c, opsID1)
	assert.GreaterOrEqual(t, Cosine(oneHot(4, 0), centroid), float64(threshold),
		"former target member must still match the merged cluster")
	assert.GreaterOrEqual(t, Cosine(oneHot(4, 1), centroid), float64(threshold),
		"former source member must now match the merged cluster")
	assert.Equal(t, opsID1, c.Assign(oneHot(4, 1)),
		"a former source member must be assigned the surviving target id")
	assert.Equal(t, 1, c.NumClusters(), "it must not start a new cluster")
}

func TestClusterer_Merge_LastSeenTakesTheMaximum(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))
	// spk_2 was seen more recently (higher tick). Merge into the older spk_1.
	_, olderLastSeen := lastSeenOf(t, c, opsID1)
	_, newerLastSeen := lastSeenOf(t, c, opsID2)
	require.Greater(t, newerLastSeen, olderLastSeen)

	require.NoError(t, c.Merge(opsID1, opsID2))

	_, merged := lastSeenOf(t, c, opsID1)
	assert.Equal(t, newerLastSeen, merged, "merged cluster keeps the more recent lastSeen")
}

// lastSeenOf returns the named cluster's id and persisted lastSeen tick.
func lastSeenOf(t *testing.T, c *Clusterer, id string) (foundID string, lastSeen int64) {
	t.Helper()
	snap := c.Snapshot()
	for i := range snap.Clusters {
		if snap.Clusters[i].ID == id {
			return snap.Clusters[i].ID, snap.Clusters[i].LastSeen
		}
	}
	t.Fatalf("cluster %s not found", id)
	return "", 0
}

func TestClusterer_Merge_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		source   string
		wantErr  error
		wantLeft int
	}{
		{name: "same id", target: opsID1, source: opsID1, wantErr: ErrSameCluster, wantLeft: 2},
		{name: "unknown target", target: opsIDAbsent, source: opsID2, wantErr: ErrUnknownCluster, wantLeft: 2},
		{name: "unknown source", target: opsID1, source: opsIDAbsent, wantErr: ErrUnknownCluster, wantLeft: 2},
		{name: "both unknown", target: opsIDAbsent2, source: opsIDAbsent, wantErr: ErrUnknownCluster, wantLeft: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := NewClusterer(0.75)
			require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
			require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))

			err := c.Merge(tt.target, tt.source)

			require.Error(t, err)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantLeft, c.NumClusters(), "a failed merge must not change the clusters")
		})
	}
}

func TestClusterer_Merge_DimensionMismatchIsRejected(t *testing.T) {
	t.Parallel()

	// Snapshot restore drops off-dimension clusters, so this state is only
	// reachable by constructing it directly — the guard is defensive.
	c := NewClusterer(0.75)
	c.clusters = []*cluster{
		{id: opsID1, centroid: []float32{1, 0, 0, 0}, count: 1},
		{id: opsID2, centroid: []float32{0, 1}, count: 1},
	}

	err := c.Merge(opsID1, opsID2)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCentroidDimensionMismatch)
	assert.Equal(t, 2, c.NumClusters())
}

func TestClusterer_Remove(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))

	require.NoError(t, c.Remove(opsID1))
	assert.Equal(t, 1, c.NumClusters())
	assert.False(t, c.HasCluster(opsID1))
	assert.True(t, c.HasCluster(opsID2))

	// Removing the same id twice, or an id that never existed, is an error.
	err := c.Remove(opsID1)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownCluster)
	require.ErrorIs(t, c.Remove(opsIDAbsent2), ErrUnknownCluster)
	assert.Equal(t, 1, c.NumClusters())
}

func TestClusterer_Remove_DoesNotReissueTheID(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))

	require.NoError(t, c.Remove(opsID2))

	// A brand new voice must continue the counter, never recycle spk_2.
	assert.Equal(t, opsID3, c.Assign(oneHot(4, 2)))
}

func TestClusterer_Merge_DoesNotReissueTheSourceID(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))

	require.NoError(t, c.Merge(opsID1, opsID2))

	assert.Equal(t, 2, c.Snapshot().NextID, "merge must leave nextID untouched")
	assert.Equal(t, opsID3, c.Assign(oneHot(4, 3)))
}

func TestClusterer_SnapshotRoundTripAfterMerge(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))
	require.NoError(t, c.Merge(opsID1, opsID2))

	before, beforeCount := centroidOf(t, c, opsID1)
	restored := NewClustererFromSnapshot(c.Snapshot())

	// Restore keeps exactly the merged state: one cluster, same centroid/count.
	assert.Equal(t, 1, restored.NumClusters())
	after, afterCount := centroidOf(t, restored, opsID1)
	assert.Equal(t, beforeCount, afterCount)
	assert.Equal(t, before, after)

	// The nextID monotonicity guard still holds: the retired spk_2 is not
	// reissued after a restart, so historic detections keep unambiguous labels.
	assert.Equal(t, 2, restored.Snapshot().NextID)
	assert.Equal(t, opsID3, restored.Assign(oneHot(4, 3)))
}

func TestClusterer_SnapshotRoundTripAfterRemove(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))
	require.Equal(t, opsID3, c.Assign(oneHot(4, 2)))
	// Forget the highest-numbered cluster: nextID must not fall back to it.
	require.NoError(t, c.Remove(opsID3))

	restored := NewClustererFromSnapshot(c.Snapshot())

	assert.Equal(t, 2, restored.NumClusters())
	assert.False(t, restored.HasCluster(opsID3))
	assert.Equal(t, 3, restored.Snapshot().NextID)
	assert.Equal(t, "spk_4", restored.Assign(oneHot(4, 3)))
}

func TestClusterer_SaveLoadAfterMerge(t *testing.T) {
	t.Parallel()

	c := NewClusterer(0.75)
	require.Equal(t, opsID1, c.Assign(oneHot(4, 0)))
	require.Equal(t, opsID2, c.Assign(oneHot(4, 1)))
	require.NoError(t, c.Merge(opsID1, opsID2))

	path := t.TempDir() + "/clusters.json"
	require.NoError(t, c.Save(path))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.NumClusters())
	assert.True(t, loaded.HasCluster(opsID1))
	assert.False(t, loaded.HasCluster(opsID2))
	assert.Equal(t, opsID3, loaded.Assign(oneHot(4, 3)))
}
