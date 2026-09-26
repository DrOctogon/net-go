package speaker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tphakala/voicewatch/internal/errors"
)

const componentSpeaker = "speaker"

// SpeakerClusterSnapshot is the serialisable state of a Clusterer. It captures
// enough to resume online clustering across process restarts: the similarity
// threshold, the next cluster-ID counter, and every cluster's running centroid.
type SpeakerClusterSnapshot struct {
	Threshold float64           `json:"threshold"`
	NextID    int               `json:"nextId"`
	Clusters  []ClusterSnapshot `json:"clusters"`
}

// ClusterSnapshot is one persisted speaker cluster.
type ClusterSnapshot struct {
	ID       string    `json:"id"`
	Centroid []float32 `json:"centroid"`
	Count    int       `json:"count"`
	// LastSeen is the clusterer's monotonic recency tick at the cluster's most
	// recent match (used for LRU eviction at MaxClusters). Absent in snapshots
	// written before eviction existed; such clusters restore as equally old.
	LastSeen int64 `json:"lastSeen,omitempty"`
}

// Snapshot returns a deep copy of the clusterer's current state. The returned
// value shares no memory with the clusterer, so it is safe to serialise or
// retain while clustering continues.
func (c *Clusterer) Snapshot() SpeakerClusterSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := SpeakerClusterSnapshot{
		Threshold: c.threshold,
		NextID:    c.nextID,
		Clusters:  make([]ClusterSnapshot, len(c.clusters)),
	}
	for i, cl := range c.clusters {
		centroid := make([]float32, len(cl.centroid))
		copy(centroid, cl.centroid)
		out.Clusters[i] = ClusterSnapshot{ID: cl.id, Centroid: centroid, Count: cl.count, LastSeen: cl.lastSeen}
	}
	return out
}

// NewClustererFromSnapshot rebuilds a Clusterer from a snapshot. Invalid or
// unsafe state is dropped or corrected defensively, since a snapshot may have
// been hand-edited or written by an older/buggy version:
//
//   - An invalid threshold (non-positive, NaN, infinite, or > 1.0, the maximum
//     possible cosine similarity) falls back to DefaultClusterThreshold (same
//     rule as NewClusterer).
//   - Clusters with an empty centroid are dropped defensively.
//   - The dimension of the first valid (non-empty-centroid) cluster wins: any
//     later cluster whose centroid has a different length is dropped too,
//     since Cosine returns 0 for mismatched lengths, making it permanently
//     unmatchable and a wasted MaxClusters slot.
//   - A non-positive member count is clamped to 1 so the incremental-mean
//     update stays well defined.
//   - nextID is raised, if needed, past the highest numeric suffix among
//     restored "spk_<n>" IDs, so a stale (too-low) snapshot NextID can never
//     mint a new ID that collides with a restored one.
//
// Centroids are copied, so the snapshot may be reused afterward.
func NewClustererFromSnapshot(s SpeakerClusterSnapshot) *Clusterer {
	c := NewClusterer(s.Threshold)
	c.nextID = s.NextID
	c.clusters = make([]*cluster, 0, len(s.Clusters))

	dim := -1
	maxSuffix := 0
	for i := range s.Clusters {
		cs := s.Clusters[i]
		if len(cs.Centroid) == 0 {
			continue
		}
		if dim == -1 {
			dim = len(cs.Centroid)
		} else if len(cs.Centroid) != dim {
			continue
		}

		centroid := make([]float32, len(cs.Centroid))
		copy(centroid, cs.Centroid)
		count := cs.Count
		if count < 1 {
			count = 1
		}
		c.clusters = append(c.clusters, &cluster{id: cs.ID, centroid: centroid, count: count, lastSeen: cs.LastSeen})
		if cs.LastSeen > c.seq {
			// Resume the recency tick past the newest restored cluster so new
			// assignments always rank as more recent than restored state.
			c.seq = cs.LastSeen
		}
		if n, ok := spkIDSuffix(cs.ID); ok && n > maxSuffix {
			maxSuffix = n
		}
	}
	if maxSuffix > c.nextID {
		c.nextID = maxSuffix
	}
	return c
}

// spkIDSuffix parses the numeric suffix of a clusterer-minted speaker ID
// (e.g. "spk_5" -> 5, true). It returns false for anything else, including
// non-conforming or malformed IDs, so restore logic can ignore them safely.
func spkIDSuffix(id string) (int, bool) {
	suffix, ok := strings.CutPrefix(id, spkIDPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// Save atomically writes the clusterer's snapshot to path as JSON. It writes to a
// temporary file in the same directory and renames it over path, so a crash mid-
// write never leaves a partially written cluster file. The file is created with
// 0600 permissions and no temp file is left behind on success or failure.
func (c *Clusterer) Save(path string) error {
	snap := c.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "marshal_speaker_clusters").
			Build()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".speaker-clusters-*.json.tmp")
	if err != nil {
		return errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "create_temp_speaker_clusters").
			Build()
	}
	tmpName := tmp.Name()
	// Remove the temp file if we bail before the rename; after a successful
	// rename tmpName no longer exists and this Remove is a harmless no-op.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "write_speaker_clusters").
			Build()
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "sync_speaker_clusters").
			Build()
	}
	if err := tmp.Close(); err != nil {
		return errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "close_speaker_clusters").
			Build()
	}
	if err := os.Rename(tmpName, path); err != nil {
		return errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "rename_speaker_clusters").
			Build()
	}
	return nil
}

// Load reads a clusterer snapshot previously written by Save and returns a
// restored Clusterer. A missing file is reported as an error that satisfies
// errors.Is(err, fs.ErrNotExist) so callers can treat "no prior state" (first
// run) distinctly from a genuine read/parse failure.
func Load(path string) (*Clusterer, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied config-derived path
	if err != nil {
		return nil, errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryFileIO).
			Context("operation", "read_speaker_clusters").
			Build()
	}

	var snap SpeakerClusterSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, errors.New(err).
			Component(componentSpeaker).
			Category(errors.CategoryValidation).
			Context("operation", "unmarshal_speaker_clusters").
			Build()
	}
	return NewClustererFromSnapshot(snap), nil
}
