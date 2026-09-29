package speaker

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"sync"

	"github.com/tphakala/voicewatch/internal/errors"
)

// Cluster-management errors, returned by Merge and Remove so callers (the API
// layer) can map them to status codes without string matching.
var (
	// ErrUnknownCluster means no cluster with the requested ID exists.
	ErrUnknownCluster = errors.NewStd("unknown speaker cluster")
	// ErrSameCluster means a merge named the same cluster as target and source.
	ErrSameCluster = errors.NewStd("cannot merge a speaker cluster into itself")
	// ErrCentroidDimensionMismatch means two clusters carry differently sized
	// centroids and cannot be averaged. Defensive: a fixed-dimension voice-print
	// model never produces this.
	ErrCentroidDimensionMismatch = errors.NewStd("speaker cluster centroids have different dimensions")
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

// HasCluster reports whether a cluster with the given ID currently exists.
// Callers that act on the result must tolerate the cluster disappearing
// afterwards (LRU eviction, a concurrent Merge/Remove).
func (c *Clusterer) HasCluster(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.indexOf(id) >= 0
}

// Merge folds the source cluster into the target: the target's centroid becomes
// the count-weighted mean of both centroids, its count becomes the sum, and its
// lastSeen the more recent of the two. The source cluster is then removed. The
// target's ID survives; the source's ID is retired and never reissued, since
// nextID is left untouched.
//
// It is the correction for the greedy online clusterer splitting one person
// across two IDs (a voice heard first over a noisy source, say).
func (c *Clusterer) Merge(targetID, sourceID string) error {
	if targetID == sourceID {
		return fmt.Errorf("%w: %s", ErrSameCluster, targetID)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	ti, si := c.indexOf(targetID), c.indexOf(sourceID)
	if ti < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownCluster, targetID)
	}
	if si < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownCluster, sourceID)
	}

	target, source := c.clusters[ti], c.clusters[si]
	if len(target.centroid) != len(source.centroid) {
		// Defensive: a fixed-dimension model never trips this, and
		// NewClustererFromSnapshot drops off-dimension clusters on restore.
		return fmt.Errorf("%w: %s has %d dimensions, %s has %d",
			ErrCentroidDimensionMismatch, targetID, len(target.centroid), sourceID, len(source.centroid))
	}

	// Count-weighted mean of the two running means, which is exactly the mean
	// of all members of both clusters. Accumulated in float64 so a large count
	// ratio does not lose the smaller cluster entirely to float32 rounding.
	total := target.count + source.count
	tw, sw := float64(target.count), float64(source.count)
	for i := range target.centroid {
		target.centroid[i] = float32((float64(target.centroid[i])*tw + float64(source.centroid[i])*sw) / float64(total))
	}
	target.count = total
	target.lastSeen = max(target.lastSeen, source.lastSeen)

	c.clusters = slices.Delete(c.clusters, si, si+1)
	return nil
}

// Remove drops the cluster with the given ID ("forget this voice"). Its ID is
// retired, not reissued: nextID is left untouched.
func (c *Clusterer) Remove(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	i := c.indexOf(id)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownCluster, id)
	}
	c.clusters = slices.Delete(c.clusters, i, i+1)
	return nil
}

// indexOf returns the index of the cluster with the given ID, or -1. The caller
// must hold c.mu.
func (c *Clusterer) indexOf(id string) int {
	return slices.IndexFunc(c.clusters, func(cl *cluster) bool { return cl.id == id })
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
