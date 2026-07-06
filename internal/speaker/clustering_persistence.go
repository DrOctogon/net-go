package speaker

import (
	"encoding/json"
	"os"
	"path/filepath"

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
		out.Clusters[i] = ClusterSnapshot{ID: cl.id, Centroid: centroid, Count: cl.count}
	}
	return out
}

// NewClustererFromSnapshot rebuilds a Clusterer from a snapshot. A non-positive
// snapshot threshold falls back to DefaultClusterThreshold (same rule as
// NewClusterer). Clusters with an empty centroid are dropped defensively; a
// non-positive member count is clamped to 1 so the incremental-mean update stays
// well defined. Centroids are copied, so the snapshot may be reused afterward.
func NewClustererFromSnapshot(s SpeakerClusterSnapshot) *Clusterer {
	c := NewClusterer(s.Threshold)
	c.nextID = s.NextID
	c.clusters = make([]*cluster, 0, len(s.Clusters))
	for i := range s.Clusters {
		cs := s.Clusters[i]
		if len(cs.Centroid) == 0 {
			continue
		}
		centroid := make([]float32, len(cs.Centroid))
		copy(centroid, cs.Centroid)
		count := cs.Count
		if count < 1 {
			count = 1
		}
		c.clusters = append(c.clusters, &cluster{id: cs.ID, centroid: centroid, count: count})
	}
	return c
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
