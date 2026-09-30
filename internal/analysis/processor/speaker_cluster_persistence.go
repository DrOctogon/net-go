// speaker_cluster_persistence.go: cross-restart persistence for voice-print
// speaker clusters. The online Clusterer is in-memory; without this, every
// restart re-numbers speakers from spk_1. Clusters are stored as a small JSON
// snapshot next to the SQLite database (the natural per-install state directory)
// and reloaded at startup so SpeakerIDs stay stable across restarts.
package processor

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// speakerClusterStateFile is the snapshot filename written beside the database.
const speakerClusterStateFile = "speaker_clusters.json"

// speakerClusterAutosaveInterval is how often learned clusters are written to
// disk between the operator-correction and shutdown saves, and so the maximum
// amount of learning an unclean exit (power loss on a Pi, OOM kill, docker kill)
// can cost. Five minutes matches pipelineStatsInterval and is a deliberate
// compromise for SD-card-backed hardware: the snapshot is a few KB, so even a
// continuously busy install writes it ~288 times a day — negligible wear — while
// a quiet one writes nothing at all, because an unchanged snapshot is skipped.
// Shorter would buy little: a brand-new speaker is a rare event, so a five-minute
// window usually holds no new clusters at all.
const speakerClusterAutosaveInterval = 5 * time.Minute

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

// persistSpeakerClusters writes the current clusters to disk. It is called at
// shutdown and immediately after an operator cluster merge/forget; failures are
// logged but never block the caller.
func (p *Processor) persistSpeakerClusters() {
	if p.speakerClusterer == nil {
		return
	}
	path, ok := p.speakerClusterStatePath()
	if !ok {
		return
	}

	if err := p.speakerClusterer.Save(path); err != nil {
		GetLogger().Warn("Failed to persist voice-print clusters",
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

// startSpeakerClusterAutosave runs a ticker that periodically persists changed
// voice-print clusters, so an unclean exit loses at most one interval of
// learning instead of everything since process start. It is additive: the
// shutdown and operator-correction saves are unaffected.
//
// The goroutine is owned by ctx — the processor's flusher lifecycle context,
// cancelled at the top of ShutdownWithContext — and returns as soon as it is
// cancelled. It is a no-op when voice-print clustering is disabled (no
// clusterer). Failures never propagate: the clusterer keeps the data in memory
// and stays dirty, so the next tick retries.
func (p *Processor) startSpeakerClusterAutosave(ctx context.Context) {
	clusterer := p.speakerClusterer
	if clusterer == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(speakerClusterAutosaveInterval)
		defer ticker.Stop()

		log := GetLogger()
		log.Info("Starting voice-print cluster autosave",
			logger.String("component", "analysis.processor.speaker"),
			logger.Duration("interval", speakerClusterAutosaveInterval),
			logger.String("operation", "speaker_cluster_autosave_startup"))

		// failing is true while the previous tick's save failed. A persistently
		// unwritable snapshot path must not emit a warning every interval
		// forever, so only the start of a failure streak warns.
		failing := false

		for {
			select {
			case <-ctx.Done():
				log.Info("Voice-print cluster autosave stopped",
					logger.String("component", "analysis.processor.speaker"),
					logger.String("operation", "speaker_cluster_autosave_shutdown"))
				return
			case <-ticker.C:
				// Resolved per tick, not captured at start, so a settings change
				// to the SQLite store takes effect without a restart.
				path, ok := p.speakerClusterStatePath()
				if !ok {
					continue
				}

				wrote, err := clusterer.SaveIfChanged(path)
				switch {
				case err != nil:
					if failing {
						log.Debug("Voice-print cluster autosave still failing; will retry",
							logger.String("component", "analysis.processor.speaker"),
							logger.String("path", path),
							logger.Error(err),
							logger.String("operation", "speaker_cluster_autosave"))
					} else {
						log.Warn("Failed to autosave voice-print clusters; keeping them in memory and retrying next interval",
							logger.String("component", "analysis.processor.speaker"),
							logger.String("path", path),
							logger.Error(err),
							logger.String("operation", "speaker_cluster_autosave"))
					}
					failing = true
				case wrote:
					failing = false
					log.Debug("Autosaved voice-print clusters to disk",
						logger.String("component", "analysis.processor.speaker"),
						logger.Int("clusters", clusterer.NumClusters()),
						logger.String("path", path),
						logger.String("operation", "speaker_cluster_autosave"))
				default:
					// Unchanged since the last save — skip the write entirely.
					failing = false
				}
			}
		}
	}()
}
