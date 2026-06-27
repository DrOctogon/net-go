package speaker

import (
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

func TestNewClustererDefaultThreshold(t *testing.T) {
	t.Parallel()
	c := NewClusterer(0)
	assert.InDelta(t, DefaultClusterThreshold, c.threshold, 1e-9)
	cNeg := NewClusterer(-1)
	assert.InDelta(t, DefaultClusterThreshold, cNeg.threshold, 1e-9)
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
