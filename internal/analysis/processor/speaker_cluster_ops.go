// speaker_cluster_ops.go: operator corrections to voice-print speaker clusters.
// The greedy online clusterer is not perfect: one person can end up split across
// two ids (heard first over a noisy source, say), and an operator may want a
// voice forgotten entirely. These are the two narrow seams the API layer calls;
// the Clusterer itself lives in internal/speaker.
package processor

import (
	"context"
	"fmt"

	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// componentSpeakerOps is the structured-logging component for speaker cluster
// management, matching the persistence code's component name.
const componentSpeakerOps = "analysis.processor.speaker"

// ErrSpeakerClusteringUnavailable means there is no in-memory clusterer to
// operate on because voice-print speaker attributes are disabled. The API maps
// it to 503: the request is well formed, the capability is simply off.
var ErrSpeakerClusteringUnavailable = errors.NewStd("voice-print speaker clustering is not enabled")

// ErrDatastoreUnavailable means the processor has no datastore, so a speaker
// relabelling cannot be persisted.
var ErrDatastoreUnavailable = errors.NewStd("datastore is not available")

// MergeSpeakerClusters folds the source cluster into the target: every detection
// labelled with sourceID is relabelled targetID, the source's speaker-name row is
// removed, and the in-memory centroids are combined (count-weighted). The target
// id survives; the source id is retired and never reissued.
//
// Ordering: both ids are validated against the live clusterer first (so an
// unknown id never causes a database write), then the database is relabelled,
// then the in-memory clusterer is merged, then the snapshot is written
// immediately rather than waiting for shutdown. A database failure therefore
// leaves the clusterer untouched and the operation fully retryable. The reverse
// order would risk a merged clusterer with unmoved rows, which no retry fixes.
//
// The check-then-merge gap is not locked across both stores: a cluster evicted
// (MaxClusters LRU) in that window makes the merge fail after the rows have
// already moved. The detections keep a stable, if cluster-less, label and the
// operation can be repeated; a cross-store transaction is not worth it for a
// hand-driven correction.
//
// Name policy: the target keeps its own display name. An unnamed target adopts
// the source's name, so merging "spk_7 (unnamed)" <- "spk_3 (Alice)" yields
// "spk_7 (Alice)" rather than silently losing the label the operator typed.
// The source's name row is always deleted.
func (p *Processor) MergeSpeakerClusters(ctx context.Context, targetID, sourceID string) error {
	if p.speakerClusterer == nil {
		return ErrSpeakerClusteringUnavailable
	}
	if p.Ds == nil {
		return ErrDatastoreUnavailable
	}

	// Validate before writing: Merge re-checks under its own lock, but doing it
	// here keeps an unknown or self-referential id from touching the database.
	if targetID == sourceID {
		return fmt.Errorf("%w: %s", speaker.ErrSameCluster, targetID)
	}
	if !p.speakerClusterer.HasCluster(targetID) {
		return fmt.Errorf("%w: %s", speaker.ErrUnknownCluster, targetID)
	}
	if !p.speakerClusterer.HasCluster(sourceID) {
		return fmt.Errorf("%w: %s", speaker.ErrUnknownCluster, sourceID)
	}

	moved, err := p.Ds.ReassignSpeakerID(ctx, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("relabel detections %s -> %s: %w", sourceID, targetID, err)
	}
	if err := p.adoptSpeakerNameOnMerge(ctx, targetID, sourceID); err != nil {
		return err
	}
	if err := p.speakerClusterer.Merge(targetID, sourceID); err != nil {
		return err
	}
	p.persistSpeakerClusters()

	GetLogger().Info("Merged voice-print speaker clusters",
		logger.String("component", componentSpeakerOps),
		logger.String("target_speaker_id", targetID),
		logger.String("source_speaker_id", sourceID),
		logger.Int64("detections_relabelled", moved),
		logger.String("operation", "merge_speaker_clusters"))
	return nil
}

// ForgetSpeakerCluster removes a voice-print cluster and its attribution: the
// cluster's detections are unlabelled (the clips themselves are kept), its
// display name is deleted, the in-memory cluster is dropped, and the snapshot is
// rewritten. The id is retired, never reissued.
//
// Same ordering rationale as MergeSpeakerClusters: database first, then memory,
// so a database failure leaves the clusterer intact and the call retryable.
func (p *Processor) ForgetSpeakerCluster(ctx context.Context, speakerID string) error {
	if p.speakerClusterer == nil {
		return ErrSpeakerClusteringUnavailable
	}
	if p.Ds == nil {
		return ErrDatastoreUnavailable
	}
	if !p.speakerClusterer.HasCluster(speakerID) {
		return fmt.Errorf("%w: %s", speaker.ErrUnknownCluster, speakerID)
	}

	cleared, err := p.Ds.ClearSpeakerID(ctx, speakerID)
	if err != nil {
		return fmt.Errorf("unlabel detections of %s: %w", speakerID, err)
	}
	// Drop the display name too: a forgotten cluster must not linger on the
	// roster, which keeps named speakers even with zero detections.
	if err := p.Ds.SetSpeakerName(ctx, speakerID, ""); err != nil {
		return fmt.Errorf("clear speaker name %s: %w", speakerID, err)
	}
	if err := p.speakerClusterer.Remove(speakerID); err != nil {
		return err
	}
	p.persistSpeakerClusters()

	GetLogger().Info("Forgot voice-print speaker cluster",
		logger.String("component", componentSpeakerOps),
		logger.String("speaker_id", speakerID),
		logger.Int64("detections_unlabelled", cleared),
		logger.String("operation", "forget_speaker_cluster"))
	return nil
}

// adoptSpeakerNameOnMerge applies the merge name policy documented on
// MergeSpeakerClusters: an unnamed target inherits the source's name, a named
// target keeps its own, and the source's name row is always deleted.
func (p *Processor) adoptSpeakerNameOnMerge(ctx context.Context, targetID, sourceID string) error {
	names, err := p.Ds.GetSpeakerNames(ctx)
	if err != nil {
		return fmt.Errorf("read speaker names: %w", err)
	}

	var targetName, sourceName string
	for i := range names {
		switch names[i].SpeakerID {
		case targetID:
			targetName = names[i].Name
		case sourceID:
			sourceName = names[i].Name
		}
	}
	if sourceName == "" {
		// No source row to delete and nothing to adopt.
		return nil
	}

	if targetName == "" {
		if err := p.Ds.SetSpeakerName(ctx, targetID, sourceName); err != nil {
			return fmt.Errorf("adopt speaker name for %s: %w", targetID, err)
		}
	}
	// An empty name deletes the row.
	if err := p.Ds.SetSpeakerName(ctx, sourceID, ""); err != nil {
		return fmt.Errorf("delete merged speaker name %s: %w", sourceID, err)
	}
	return nil
}
