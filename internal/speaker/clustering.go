package speaker

import (
	"math"
	"strconv"
	"sync"
)

// DefaultClusterThreshold is the cosine-similarity floor above which a voice
// print is treated as the same speaker as an existing cluster. Voice-print
// models typically separate distinct speakers well above this; the value should
// be recalibrated once a real model is wired in.
const DefaultClusterThreshold = 0.75

// maxCosineSimilarity is the maximum value Cosine can return. A threshold
// above this can never be met, which would silently break clustering (every
// voice becomes a new cluster).
const maxCosineSimilarity = 1.0

// spkIDPrefix prefixes every clusterer-minted speaker ID (e.g. "spk_1").
const spkIDPrefix = "spk_"

// MaxClusters caps how many speaker clusters are retained. When a new voice
// arrives at the cap, the least-recently-seen cluster is evicted (its ID is
// never reused). Bounds the O(N) cosine scan in AssignWithNovelty and the
// snapshot size.
// ponytail: fixed cap + LRU eviction; make it a config knob and/or add an ANN
// index only if real deployments exceed 64 recurring speakers.
const MaxClusters = 64

// Clusterer performs online, greedy voice-print clustering to assign a stable
// speaker ID to detections. Each Assign compares a new embedding against the
// running centroid of every known cluster and either joins the most-similar
// cluster (cosine >= Threshold) or starts a new one.
//
// It is a dependency-free in-memory primitive, deliberately simple so it can be
// unit tested before a real voice-print model exists. Its state can be persisted
// across restarts via Snapshot/Save and restored with Load/NewClustererFromSnapshot
// (see clustering_persistence.go); cluster eviction (capping or ageing out
// clusters) can be layered on later. Safe for concurrent use.
type Clusterer struct {
	mu        sync.Mutex
	threshold float64
	clusters  []*cluster
	nextID    int
	// seq is a monotonic tick, incremented per assignment, used to track each
	// cluster's recency for LRU eviction at MaxClusters.
	seq int64
}

type cluster struct {
	id       string
	centroid []float32 // running mean of member embeddings
	count    int
	lastSeen int64 // Clusterer.seq value of the most recent match/creation
}

// NewClusterer returns a Clusterer using the given cosine-similarity threshold.
// A threshold that is <= 0, NaN, infinite, or greater than the maximum
// possible cosine similarity (1.0) falls back to DefaultClusterThreshold —
// any of those would either break clustering (every voice becomes a new
// cluster) or make the threshold unmarshalable when saving a snapshot.
func NewClusterer(threshold float64) *Clusterer {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold <= 0 || threshold > maxCosineSimilarity {
		threshold = DefaultClusterThreshold
	}
	return &Clusterer{threshold: threshold}
}

// Assign returns the speaker ID for the given voice-print embedding, creating a
// new cluster when no existing one is similar enough. It returns "" for an empty
// embedding (nothing to cluster). The embedding is copied before being retained,
// so the caller may reuse its slice.
func (c *Clusterer) Assign(embedding []float32) string {
	id, _ := c.AssignWithNovelty(embedding)
	return id
}

// AssignWithNovelty is Assign plus a novelty flag: isNew is true when the
// embedding did not match any existing cluster and a new one was created
// (an unknown voice). It is false for an empty embedding and for matches
// against known clusters, including clusters restored from a snapshot.
func (c *Clusterer) AssignWithNovelty(embedding []float32) (id string, isNew bool) {
	if len(embedding) == 0 {
		return "", false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Pick the most-similar cluster whose similarity meets the threshold.
	bestIdx := -1
	bestSim := c.threshold
	for i, cl := range c.clusters {
		sim := Cosine(embedding, cl.centroid)
		if sim >= bestSim {
			bestSim = sim
			bestIdx = i
		}
	}

	c.seq++

	if bestIdx >= 0 {
		c.clusters[bestIdx].update(embedding)
		c.clusters[bestIdx].lastSeen = c.seq
		return c.clusters[bestIdx].id, false
	}

	// No match: start a new cluster with a fresh deterministic ID, evicting
	// the least-recently-seen cluster when at the cap (see MaxClusters).
	c.nextID++
	id = spkIDPrefix + strconv.Itoa(c.nextID)
	centroid := make([]float32, len(embedding))
	copy(centroid, embedding)
	fresh := &cluster{id: id, centroid: centroid, count: 1, lastSeen: c.seq}

	if len(c.clusters) >= MaxClusters {
		lru := 0
		for i, cl := range c.clusters {
			if cl.lastSeen < c.clusters[lru].lastSeen {
				lru = i
			}
		}
		c.clusters[lru] = fresh
	} else {
		c.clusters = append(c.clusters, fresh)
	}
	return id, true
}

// NumClusters returns the number of distinct speakers seen so far.
func (c *Clusterer) NumClusters() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.clusters)
}

// update folds a new embedding into the cluster's running-mean centroid using
// the incremental-mean formula (mean += (x - mean) / n).
func (cl *cluster) update(embedding []float32) {
	if len(embedding) != len(cl.centroid) {
		// Defensive: a fixed-dimension model never trips this. Ignore a
		// dimension mismatch rather than corrupt the centroid.
		return
	}
	cl.count++
	n := float32(cl.count)
	for i := range cl.centroid {
		cl.centroid[i] += (embedding[i] - cl.centroid[i]) / n
	}
}
