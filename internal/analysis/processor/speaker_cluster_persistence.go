// speaker_cluster_persistence.go: cross-restart persistence for voice-print
// speaker clusters. The online Clusterer is in-memory; without this, every
// restart re-numbers speakers from spk_1. Clusters are stored as a small JSON
// snapshot next to the SQLite database (the natural per-install state directory)
// and reloaded at startup so SpeakerIDs stay stable across restarts.
package processor

import (
	"io/fs"
	"path/filepath"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// speakerClusterStateFile is the snapshot filename written beside the database.
const speakerClusterStateFile = "speaker_clusters.json"

// speakerClusterStatePath returns the on-disk path for persisted voice-print
// clusters and whether persistence is possible. Clusters live next to the SQLite
// database; when SQLite is not the configured store there is no stable state
// directory, so persistence is skipped (the second return is false).
func (p *Processor) speakerClusterStatePath() (string, bool) {
	s := p.currentSettings()
	if !s.Output.SQLite.Enabled || s.Output.SQLite.Path == "" {
		return "", false
	}
	dir := conf.GetBasePath(filepath.Dir(s.Output.SQLite.Path))
	return filepath.Join(dir, speakerClusterStateFile), true
}

// restoreSpeakerClusters replaces p.speakerClusterer with one rebuilt from the
// persisted snapshot, when a readable snapshot exists. A missing file (first run)
// or any load error leaves the freshly created in-memory clusterer in place, so
// clustering always continues — persistence is best-effort and never fatal.
func (p *Processor) restoreSpeakerClusters() {
	if p.speakerClusterer == nil {
		return
	}
	path, ok := p.speakerClusterStatePath()
	if !ok {
		return
	}

	restored, err := speaker.Load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			GetLogger().Debug("No persisted voice-print clusters found (first run)",
				logger.String("component", "analysis.processor.speaker"),
				logger.String("path", path),
				logger.String("operation", "restore_speaker_clusters"))
			return
		}
		GetLogger().Warn("Failed to load persisted voice-print clusters; starting fresh",
			logger.String("component", "analysis.processor.speaker"),
			logger.String("path", path),
			logger.Error(err),
			logger.String("operation", "restore_speaker_clusters"))
		return
	}

	p.speakerClusterer = restored
	GetLogger().Info("Restored voice-print clusters from disk",
		logger.String("component", "analysis.processor.speaker"),
		logger.Int("clusters", restored.NumClusters()),
		logger.String("path", path),
		logger.String("operation", "restore_speaker_clusters"))
}

// persistSpeakerClusters writes the current clusters to disk. It is called during
// shutdown; failures are logged but never block shutdown.
func (p *Processor) persistSpeakerClusters() {
	if p.speakerClusterer == nil {
		return
	}
	path, ok := p.speakerClusterStatePath()
	if !ok {
		return
	}

	if err := p.speakerClusterer.Save(path); err != nil {
		GetLogger().Warn("Failed to persist voice-print clusters during shutdown",
			logger.String("component", "analysis.processor.speaker"),
			logger.String("path", path),
			logger.Error(err),
			logger.String("operation", "persist_speaker_clusters"))
		return
	}
	GetLogger().Info("Persisted voice-print clusters to disk",
		logger.String("component", "analysis.processor.speaker"),
		logger.Int("clusters", p.speakerClusterer.NumClusters()),
		logger.String("path", path),
		logger.String("operation", "persist_speaker_clusters"))
}
