// ttl_cache.go: minimal stoppable TTL cache for detection query results.
//
// Replaces patrickmn/go-cache for the detection cache: that library's cleanup
// janitor goroutine cannot be stopped, so every controller restart leaked one.
// This implementation is the smallest thing that covers the controller's
// needs (Get/Set with a single TTL, Flush, and a Stop that terminates the
// janitor) — not a general-purpose cache.
package api

import (
	"sync"
	"time"
)

type ttlEntry struct {
	value   any
	expires time.Time
}

// ttlCache is a concurrency-safe string-keyed cache where every entry shares
// one fixed TTL. A background janitor evicts expired entries every cleanup
// interval and exits when Stop is called.
type ttlCache struct {
	mu       sync.RWMutex
	entries  map[string]ttlEntry
	ttl      time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

// newTTLCache returns a running cache whose janitor wakes every cleanup
// interval. Callers own the lifecycle: Stop must be called on shutdown.
func newTTLCache(ttl, cleanup time.Duration) *ttlCache {
	c := &ttlCache{
		entries: make(map[string]ttlEntry),
		ttl:     ttl,
		stop:    make(chan struct{}),
	}
	go c.janitor(cleanup)
	return c
}

// Get returns the live value for key. Expired entries read as absent (the
// janitor removes them lazily).
func (c *ttlCache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

// Set stores value under key with the cache's fixed TTL.
func (c *ttlCache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = ttlEntry{value: value, expires: time.Now().Add(c.ttl)}
}

// Flush discards all entries; the cache remains usable.
func (c *ttlCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]ttlEntry)
}

// Stop terminates the janitor goroutine. Safe to call more than once; the
// cache itself stays readable/writable afterward (entries still expire on
// read, they just stop being evicted in the background).
func (c *ttlCache) Stop() {
	c.stopOnce.Do(func() { close(c.stop) })
}

func (c *ttlCache) janitor(cleanup time.Duration) {
	ticker := time.NewTicker(cleanup)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for k, e := range c.entries {
				if now.After(e.expires) {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		}
	}
}
