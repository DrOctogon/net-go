package speaker

import (
	"strconv"
	"sync"
)

// DefaultClusterThreshold is the cosine-similarity floor above which a voice
// print is treated as the same speaker as an existing cluster. Voice-print
// models typically separate distinct speakers well above this; the value should
// be recalibrated once a real model is wired in.
const DefaultClusterThreshold = 0.75

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
}

type cluster struct {
	id       string
	centroid []float32 // running mean of member embeddings
	count    int
}

// NewClusterer returns a Clusterer using the given cosine-similarity threshold.
// A threshold <= 0 falls back to DefaultClusterThreshold.
func NewClusterer(threshold float64) *Clusterer {
	if threshold <= 0 {
		threshold = DefaultClusterThreshold
	}
	return &Clusterer{threshold: threshold}
}

// Assign returns the speaker ID for the given voice-print embedding, creating a
// new cluster when no existing one is similar enough. It returns "" for an empty
// embedding (nothing to cluster). The embedding is copied before being retained,
// so the caller may reuse its slice.
func (c *Clusterer) Assign(embedding []float32) string {
	if len(embedding) == 0 {
		return ""
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

	if bestIdx >= 0 {
		c.clusters[bestIdx].update(embedding)
		return c.clusters[bestIdx].id
	}

	// No match: start a new cluster with a fresh deterministic ID.
	c.nextID++
	id := "spk_" + strconv.Itoa(c.nextID)
	centroid := make([]float32, len(embedding))
	copy(centroid, embedding)
	c.clusters = append(c.clusters, &cluster{id: id, centroid: centroid, count: 1})
	return id
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
