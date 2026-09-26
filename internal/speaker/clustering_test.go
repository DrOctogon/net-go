package speaker

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClustererAssign(t *testing.T) {
	t.Parallel()

	t.Run("empty embedding returns no id", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		assert.Empty(t, c.Assign(nil))
		assert.Empty(t, c.Assign([]float32{}))
		assert.Equal(t, 0, c.NumClusters())
	})

	t.Run("first embedding creates a cluster", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		id := c.Assign([]float32{1, 0, 0})
		assert.Equal(t, "spk_1", id)
		assert.Equal(t, 1, c.NumClusters())
	})

	t.Run("identical embedding rejoins same cluster", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		first := c.Assign([]float32{1, 0, 0})
		second := c.Assign([]float32{1, 0, 0})
		assert.Equal(t, first, second)
		assert.Equal(t, 1, c.NumClusters())
	})

	t.Run("orthogonal embeddings make distinct clusters", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		a := c.Assign([]float32{1, 0, 0})
		b := c.Assign([]float32{0, 1, 0})
		assert.NotEqual(t, a, b)
		assert.Equal(t, "spk_1", a)
		assert.Equal(t, "spk_2", b)
		assert.Equal(t, 2, c.NumClusters())
	})

	t.Run("similar-above-threshold joins, dissimilar splits", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.9)
		base := c.Assign([]float32{1, 0, 0})
		// cosine([1,0,0],[10,1,0]) ~= 0.995 >= 0.9 -> same cluster
		near := c.Assign([]float32{10, 1, 0})
		assert.Equal(t, base, near)
		assert.Equal(t, 1, c.NumClusters())
		// cosine([1,0,0],[1,1,0]) ~= 0.707 < 0.9 -> new cluster
		far := c.Assign([]float32{1, 1, 0})
		assert.NotEqual(t, base, far)
		assert.Equal(t, 2, c.NumClusters())
	})

	t.Run("assigns to the most similar of several clusters", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.5)
		x := c.Assign([]float32{1, 0, 0})
		y := c.Assign([]float32{0, 1, 0})
		require.NotEqual(t, x, y)
		// Closer to the x-axis cluster than the y-axis cluster.
		got := c.Assign([]float32{0.9, 0.4, 0})
		assert.Equal(t, x, got)
		assert.Equal(t, 2, c.NumClusters())
	})
}

func TestNewClustererThresholdValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		threshold float64
		want      float64
	}{
		{"NewClusterer: zero threshold falls back to default", 0, DefaultClusterThreshold},
		{"NewClusterer: negative threshold falls back to default", -1, DefaultClusterThreshold},
		{"NewClusterer: NaN threshold falls back to default", math.NaN(), DefaultClusterThreshold},
		{"NewClusterer: positive-infinity threshold falls back to default", math.Inf(1), DefaultClusterThreshold},
		{"NewClusterer: negative-infinity threshold falls back to default", math.Inf(-1), DefaultClusterThreshold},
		{"NewClusterer: threshold above cosine max falls back to default", 1.5, DefaultClusterThreshold},
		{"NewClusterer: threshold exactly at cosine max is kept", 1.0, 1.0},
		{"NewClusterer: valid threshold is kept unchanged", 0.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := NewClusterer(tt.threshold)
			assert.InDelta(t, tt.want, c.threshold, 1e-9)
		})
	}
}

func TestClustererConcurrentAssign(t *testing.T) {
	t.Parallel()
	c := NewClusterer(0.75)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			// All identical -> must collapse to a single cluster regardless of
			// interleaving. Exercised with -race.
			_ = c.Assign([]float32{1, 0, 0})
		})
	}
	wg.Wait()
	assert.Equal(t, 1, c.NumClusters())
}

func TestClustererAssignWithNovelty(t *testing.T) {
	t.Parallel()

	t.Run("empty embedding is not novel", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		id, isNew := c.AssignWithNovelty(nil)
		assert.Empty(t, id)
		assert.False(t, isNew)
	})

	t.Run("first embedding creates a new cluster", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		id, isNew := c.AssignWithNovelty([]float32{1, 0, 0})
		assert.Equal(t, "spk_1", id)
		assert.True(t, isNew)
	})

	t.Run("similar embedding reuses the cluster and is not novel", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		first, isNew := c.AssignWithNovelty([]float32{1, 0, 0})
		require.True(t, isNew)
		second, isNew := c.AssignWithNovelty([]float32{0.99, 0.01, 0})
		assert.Equal(t, first, second)
		assert.False(t, isNew)
	})

	t.Run("dissimilar embedding creates another new cluster", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		_, isNew := c.AssignWithNovelty([]float32{1, 0, 0})
		require.True(t, isNew)
		id, isNew := c.AssignWithNovelty([]float32{0, 1, 0})
		assert.Equal(t, "spk_2", id)
		assert.True(t, isNew)
	})
}

func TestClustererCapAndEviction(t *testing.T) {
	t.Parallel()

	// Orthogonal unit embeddings guarantee zero cosine similarity, so every
	// distinct index creates a new cluster.
	embed := func(dim, hot int) []float32 {
		e := make([]float32, dim)
		e[hot] = 1
		return e
	}

	t.Run("cluster count never exceeds the cap", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		for i := range MaxClusters + 10 {
			c.Assign(embed(MaxClusters+10, i))
		}
		assert.Equal(t, MaxClusters, c.NumClusters())
	})

	t.Run("least recently seen cluster is evicted", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		dim := MaxClusters + 2
		firstID := c.Assign(embed(dim, 0)) // oldest...
		for i := 1; i < MaxClusters; i++ {
			c.Assign(embed(dim, i))
		}
		// ...but touch the first cluster again so cluster #2 becomes LRU.
		gotFirst := c.Assign(embed(dim, 0))
		require.Equal(t, firstID, gotFirst)

		// Cap reached; a new voice evicts the LRU (cluster for index 1).
		newID, isNew := c.AssignWithNovelty(embed(dim, MaxClusters))
		assert.True(t, isNew)
		assert.NotEmpty(t, newID)
		assert.Equal(t, MaxClusters, c.NumClusters())

		// The recently-touched first cluster survived eviction.
		stillFirst, isNewAgain := c.AssignWithNovelty(embed(dim, 0))
		assert.Equal(t, firstID, stillFirst)
		assert.False(t, isNewAgain)

		// The evicted voice (index 1) now clusters as NEW with a fresh ID —
		// evicted IDs are never reused.
		reassigned, novel := c.AssignWithNovelty(embed(dim, 1))
		assert.True(t, novel)
		assert.NotEqual(t, "spk_2", reassigned)
	})

	t.Run("lastSeen survives snapshot round trip", func(t *testing.T) {
		t.Parallel()
		c := NewClusterer(0.75)
		c.Assign([]float32{1, 0})
		c.Assign([]float32{0, 1})
		c.Assign([]float32{1, 0}) // touch first again

		restored := NewClustererFromSnapshot(c.Snapshot())
		// Same match behavior after restore.
		id, isNew := restored.AssignWithNovelty([]float32{1, 0})
		assert.Equal(t, "spk_1", id)
		assert.False(t, isNew)
	})
}
