package api

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTTLCache(t *testing.T) {
	t.Parallel()

	t.Run("set then get returns value", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			c := newTTLCache(time.Minute, time.Hour)
			defer c.Stop()

			c.Set("k", 42)
			v, ok := c.Get("k")
			require.True(t, ok)
			assert.Equal(t, 42, v)
		})
	})

	t.Run("entries expire on read after TTL", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			c := newTTLCache(time.Minute, time.Hour)
			defer c.Stop()

			c.Set("k", "v")
			time.Sleep(2 * time.Minute) // fake time inside synctest bubble
			_, ok := c.Get("k")
			assert.False(t, ok)
		})
	})

	t.Run("janitor evicts expired entries in the background", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			c := newTTLCache(time.Minute, 5*time.Minute)
			defer c.Stop()

			c.Set("k", "v")
			time.Sleep(6 * time.Minute)
			synctest.Wait()

			c.mu.RLock()
			_, present := c.entries["k"]
			c.mu.RUnlock()
			assert.False(t, present, "janitor should have deleted the expired entry")
		})
	})

	t.Run("stop terminates the janitor and is idempotent", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			c := newTTLCache(time.Minute, time.Minute)
			c.Stop()
			c.Stop() // second call must not panic

			// Cache stays usable after Stop.
			c.Set("k", 1)
			_, ok := c.Get("k")
			assert.True(t, ok)
			// synctest fails the test if the janitor goroutine were still
			// running when the bubble exits, so reaching the end proves Stop
			// terminated it.
		})
	})

	t.Run("flush clears all entries", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			c := newTTLCache(time.Minute, time.Hour)
			defer c.Stop()

			c.Set("a", 1)
			c.Set("b", 2)
			c.Flush()
			_, okA := c.Get("a")
			_, okB := c.Get("b")
			assert.False(t, okA)
			assert.False(t, okB)
		})
	})
}
