// policy_reconcile.go - reconciliation sweep for drift between detection rows
// and the clip files they reference.
//
// Two kinds of drift accumulate over time:
//
//   - Dangling references: a detection row names a clip whose file is gone
//     (retention or a user deleted it, a volume was remounted, a database was
//     restored without its clips). The UI then offers audio and spectrograms
//     that 404.
//   - Orphan files: a clip file on disk that no detection row references (a
//     crash between writing the file and committing the row, a failed save, a
//     partial restore). The retention policies key off database rows, so
//     nothing else will ever reclaim these.
//
// The sweep detects both and reports them. It is deliberately conservative
// about acting:
//
//   - Clearing a dangling reference requires the file to be *definitively*
//     absent (errors.Is(err, fs.ErrNotExist)) AND no file of that name to
//     exist anywhere under the clip directory. Any other stat error means the
//     file may simply be unreachable right now (permissions, a volume mid
//     remount, an NFS hiccup) and the reference is left alone.
//   - Deleting orphan files is OFF by default and requires the explicit
//     Retention.DeleteOrphanClips opt-in.
//   - Sanity guards (see reconcileGuard) refuse to act at all when the
//     comparison looks like an infrastructure failure rather than real drift.
//
// All filesystem access goes through an os.Root sandbox rooted at the clip
// directory, so a symlink or a crafted clip_name cannot reach outside it.
package diskmanager

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/logger"
)

// reconcilePolicy is the policy label the sweep reports under in logs and
// metrics, alongside the existing "age" and "usage" policies.
const reconcilePolicy = "reconcile"

// ReconcileInterval is the minimum wall time between reconciliation sweeps.
// The sweep walks the whole clip directory and pages the whole clip reference
// table, so it runs far less often than the retention check interval.
const ReconcileInterval = 6 * time.Hour

// clipRefPageSize bounds how many clip references are read from the database
// per query, so a library with 100k clips is never loaded in one result set.
const clipRefPageSize = 500

// maxDriftFraction is the sanity bound for the reconciliation sweep. When more
// than this fraction of clip references look missing, or more than this
// fraction of clip files look unreferenced, the comparison is far more likely
// to be an infrastructure failure (unmounted volume, restored database,
// mismatched clip_name layout) than real drift, and the sweep refuses to act.
const maxDriftFraction = 0.5

// maxOrphanDeletionErrors is the error budget for a single orphan deletion run.
const maxOrphanDeletionErrors = 10

// Guard reasons reported in ReconcileResult.GuardTripped. An empty value means
// no guard tripped and the sweep was allowed to act.
const (
	guardEmptyClipDir = "clip_directory_empty"
	guardMassMissing  = "missing_fraction_exceeded"
	guardMassOrphan   = "orphan_fraction_exceeded"
)

// Metric action labels for RecordFileProcessed under the reconcile policy.
const (
	actionDanglingRef  = "dangling_ref"
	actionUnreadable   = "ref_unreadable"
	actionInvalidRef   = "ref_invalid_path"
	actionOrphanFile   = "orphan_file"
	reasonOrphanDelete = "orphan clip file with no detection row"
)

// ClipNameLister is the paged clip-reference query surface the reconciliation
// sweep needs. Both datastore implementations (the legacy GORM notes table and
// the normalized v2 detections table) satisfy it.
type ClipNameLister interface {
	// ListClipNames returns up to limit non-empty clip names in a stable
	// order, skipping the first offset rows. An empty result means the end of
	// the table has been reached.
	ListClipNames(ctx context.Context, limit, offset int) ([]string, error)
}

// ReconcileStore is the datastore surface required by ReconcileClips: the
// existing retention interface plus paged clip-reference listing.
type ReconcileStore interface {
	Interface
	ClipNameLister
}

// ReconcileResult reports what a reconciliation sweep found and what it did.
type ReconcileResult struct {
	// Err is set when the sweep could not complete (context cancellation,
	// unreadable clip directory, failed reference query).
	Err error
	// GuardTripped names the sanity guard that refused to act, or is empty.
	GuardTripped string
	// ClipRefsChecked is the number of non-empty clip references examined.
	ClipRefsChecked int
	// DanglingRefs is the number of references whose file is definitively gone.
	DanglingRefs int
	// UnreadableRefs is the number of references left alone because their
	// file could not be stat'ed for a reason other than "does not exist".
	UnreadableRefs int
	// RefsCleared is the number of database rows whose clip reference was
	// cleared. Zero when a guard tripped.
	RefsCleared int64
	// OrphanFiles is the number of clip files no detection row references.
	OrphanFiles int
	// OrphanBytes is the disk space those orphan files occupy.
	OrphanBytes int64
	// OrphansDeleted is the number of orphan files removed. Always zero unless
	// Retention.DeleteOrphanClips is enabled.
	OrphansDeleted int
	// BytesFreed is the disk space reclaimed by orphan deletion.
	BytesFreed int64
}

// clipRefScan is the outcome of paging through the database's clip references.
type clipRefScan struct {
	// dbBase is the set of basenames referenced by the database, used to
	// decide which files on disk are orphans.
	dbBase map[string]struct{}
	// dangling holds the clip names, exactly as stored, whose file is
	// definitively absent.
	dangling []string
	checked  int
	skipped  int
}

// ReconcileClips detects drift between detection rows and clip files on disk.
//
// It reports both directions (dangling references and orphan files) and, when
// the sanity guards permit, clears provably dangling references. Orphan files
// are only deleted when Retention.DeleteOrphanClips is explicitly enabled;
// detection and reporting is the default behaviour.
//
// The sweep respects ctx cancellation at every page and every deletion.
func ReconcileClips(ctx context.Context, db ReconcileStore) ReconcileResult {
	log := GetLogger()
	settings := conf.Setting()
	baseDir := strings.TrimSpace(settings.Realtime.Audio.Export.Path)
	retention := settings.Realtime.Audio.Export.Retention

	if baseDir == "" {
		log.Debug("Skipping clip reconciliation: export path not configured",
			logger.String("policy", reconcilePolicy))
		return ReconcileResult{}
	}

	// Sandbox every filesystem access to the clip directory: a symlink or a
	// crafted clip_name cannot escape an os.Root.
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return ReconcileResult{Err: errors.New(err).
			Component("diskmanager").
			Category(errors.CategoryFileIO).
			Context("policy", reconcilePolicy).
			Context("operation", "open_clip_root").
			Build()}
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			log.Warn("Failed to close clip directory handle",
				logger.String("policy", reconcilePolicy),
				logger.Error(closeErr))
		}
	}()

	startTime := time.Now()

	// Walk the clip directory once. GetAudioFilesContext already skips temp
	// files and hidden directories, marks locked clips, and honours ctx.
	files, err := GetAudioFilesContext(ctx, baseDir, allowedFileTypes, db)
	if err != nil {
		return ReconcileResult{Err: err}
	}

	diskBase := make(map[string]struct{}, len(files))
	for i := range files {
		diskBase[filepath.Base(files[i].Path)] = struct{}{}
	}

	scan, err := scanClipRefs(ctx, db, root, diskBase)
	if err != nil {
		return ReconcileResult{Err: err}
	}

	orphans := findOrphans(root, baseDir, files, scan.dbBase)

	result := ReconcileResult{
		ClipRefsChecked: scan.checked,
		DanglingRefs:    len(scan.dangling),
		UnreadableRefs:  scan.skipped,
		OrphanFiles:     len(orphans),
	}
	for _, orphan := range orphans {
		result.OrphanBytes += orphan.Size
	}

	recordReconcileMetrics(&result)

	if reason := reconcileGuard(&result, len(files)); reason != "" {
		result.GuardTripped = reason
		log.Error("Clip reconciliation refused to act: drift looks like an infrastructure failure",
			logger.String("policy", reconcilePolicy),
			logger.String("guard", reason),
			logger.Int("clip_refs_checked", result.ClipRefsChecked),
			logger.Int("dangling_refs", result.DanglingRefs),
			logger.Int("orphan_files", result.OrphanFiles),
			logger.Int("clip_files_on_disk", len(files)))
		if m := getMetrics(); m != nil {
			m.RecordCleanupError(reconcilePolicy, reason)
		}
		return result
	}

	// Clearing the reference is safe: the file is provably gone, so the
	// reference is already useless and only makes the UI serve 404s. Speech
	// derived data is scrubbed alongside it when enabled, exactly as it is
	// when retention deletes the clip itself.
	if len(scan.dangling) > 0 {
		result.RefsCleared = clearClipRefs(db, scan.dangling, reconcilePolicy, retention.ScrubSpeechData)
	}

	if retention.DeleteOrphanClips && len(orphans) > 0 {
		result.OrphansDeleted, result.BytesFreed = deleteOrphans(ctx, orphans, retention.KeepSpectrograms)
	}

	log.Info("Clip reconciliation sweep completed",
		logger.String("policy", reconcilePolicy),
		logger.Int("clip_refs_checked", result.ClipRefsChecked),
		logger.Int("dangling_refs", result.DanglingRefs),
		logger.Int64("refs_cleared", result.RefsCleared),
		logger.Int("unreadable_refs", result.UnreadableRefs),
		logger.Int("orphan_files", result.OrphanFiles),
		logger.Int64("orphan_bytes", result.OrphanBytes),
		logger.Int("orphans_deleted", result.OrphansDeleted),
		logger.Int64("bytes_freed", result.BytesFreed),
		logger.Bool("orphan_deletion_enabled", retention.DeleteOrphanClips),
		logger.Int64("duration_ms", time.Since(startTime).Milliseconds()))

	return result
}

// scanClipRefs pages through the database's clip references, classifying each
// as present, definitively absent, or unreadable. It never mutates anything.
func scanClipRefs(ctx context.Context, db ClipNameLister, root *os.Root, diskBase map[string]struct{}) (clipRefScan, error) {
	scan := clipRefScan{dbBase: make(map[string]struct{})}
	log := GetLogger()

	for offset := 0; ; offset += clipRefPageSize {
		if err := ctx.Err(); err != nil {
			return scan, err
		}

		names, err := db.ListClipNames(ctx, clipRefPageSize, offset)
		if err != nil {
			return scan, errors.New(err).
				Component("diskmanager").
				Category(errors.CategoryDatabase).
				Context("policy", reconcilePolicy).
				Context("operation", "list_clip_names").
				Context("offset", offset).
				Build()
		}
		if len(names) == 0 {
			return scan, nil
		}

		for _, name := range names {
			scan.checked++
			classifyClipRef(root, name, diskBase, &scan, log)
		}

		// A short page means the end of the table.
		if len(names) < clipRefPageSize {
			return scan, nil
		}
	}
}

// classifyClipRef records one clip reference into scan: always into dbBase (so
// the matching file is not mistaken for an orphan), and into dangling only
// when the file is definitively absent.
func classifyClipRef(root *os.Root, name string, diskBase map[string]struct{}, scan *clipRefScan, log logger.Logger) {
	rel := filepath.Clean(filepath.FromSlash(name))
	if !filepath.IsLocal(rel) {
		// A clip reference that is absolute, escapes the clip directory, or
		// names a Windows reserved device is not something this sweep acts on.
		scan.skipped++
		log.Warn("Skipping clip reference with non-local path",
			logger.String("policy", reconcilePolicy),
			logger.String("clip_name", name))
		if m := getMetrics(); m != nil {
			m.RecordFileProcessed(reconcilePolicy, actionInvalidRef)
		}
		return
	}

	base := filepath.Base(rel)
	scan.dbBase[base] = struct{}{}

	// A file of this name exists somewhere under the clip directory. Some
	// installs store clip_name with a different directory prefix than the
	// current export layout, so treat that as "present" rather than risk
	// clearing a reference to a file that is right there.
	if _, onDisk := diskBase[base]; onDisk {
		return
	}

	_, statErr := root.Stat(rel)
	switch {
	case statErr == nil:
		// Present but not picked up by the walk (unusual extension, hidden
		// directory). Not dangling.
	case errors.Is(statErr, fs.ErrNotExist):
		scan.dangling = append(scan.dangling, name)
	default:
		// Permissions, a volume mid-remount, an NFS hiccup: the file may well
		// be there. Never act on this.
		scan.skipped++
		log.Warn("Could not determine whether clip file exists; leaving reference untouched",
			logger.String("policy", reconcilePolicy),
			logger.String("clip_name", name),
			logger.Error(statErr))
		if m := getMetrics(); m != nil {
			m.RecordFileProcessed(reconcilePolicy, actionUnreadable)
		}
	}
}

// findOrphans returns the clip files that no database reference points at.
// Locked clips, symlinks and non-regular files are never reported, so they can
// never be deleted as orphans.
func findOrphans(root *os.Root, baseDir string, files []FileInfo, dbBase map[string]struct{}) []*FileInfo {
	var orphans []*FileInfo

	for i := range files {
		file := &files[i]
		if file.Locked {
			continue
		}
		if _, referenced := dbBase[filepath.Base(file.Path)]; referenced {
			continue
		}

		rel, err := filepath.Rel(baseDir, file.Path)
		if err != nil {
			continue
		}
		rel = filepath.Clean(rel)
		if !filepath.IsLocal(rel) {
			continue
		}

		// Lstat through the sandbox: a symlink (which would delete something
		// outside the clip directory's intent) or a special file is skipped.
		info, err := root.Lstat(rel)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		orphans = append(orphans, file)
	}

	return orphans
}

// reconcileGuard decides whether the observed drift is plausible enough to act
// on. It returns the tripped guard's reason, or an empty string when the sweep
// may proceed.
func reconcileGuard(result *ReconcileResult, diskFileCount int) string {
	// The clip directory holds no audio at all while the database references
	// clips: an unmounted or wiped volume, not a library that vanished.
	if result.ClipRefsChecked > 0 && diskFileCount == 0 {
		return guardEmptyClipDir
	}

	if result.ClipRefsChecked > 0 &&
		float64(result.DanglingRefs)/float64(result.ClipRefsChecked) > maxDriftFraction {
		return guardMassMissing
	}

	// Most files unreferenced means the stored clip names do not describe this
	// disk layout, which invalidates the dangling verdicts too.
	if diskFileCount > 0 &&
		float64(result.OrphanFiles)/float64(diskFileCount) > maxDriftFraction {
		return guardMassOrphan
	}

	return ""
}

// deleteOrphans removes orphan clip files. Only ever called under the explicit
// Retention.DeleteOrphanClips opt-in.
func deleteOrphans(ctx context.Context, orphans []*FileInfo, keepSpectrograms bool) (deleted int, bytesFreed int64) {
	log := GetLogger()
	errorCount := 0

	for _, orphan := range orphans {
		if err := ctx.Err(); err != nil {
			log.Info("Orphan deletion interrupted",
				logger.String("policy", reconcilePolicy),
				logger.Int("files_deleted", deleted))
			return deleted, bytesFreed
		}

		if deleted >= maxDeletionsPerRun {
			log.Debug("Reached maximum number of deletions for orphan cleanup",
				logger.String("policy", reconcilePolicy),
				logger.Int("max_deletions", maxDeletionsPerRun))
			return deleted, bytesFreed
		}

		size := orphan.Size
		if err := deleteFileAndOptionalSpectrogram(orphan, reasonOrphanDelete, keepSpectrograms, reconcilePolicy); err != nil {
			if stop, _ := handleDeletionErrorInLoop(orphan.Path, err, &errorCount, maxOrphanDeletionErrors, reconcilePolicy); stop {
				return deleted, bytesFreed
			}
			continue
		}

		deleted++
		bytesFreed += size
	}

	return deleted, bytesFreed
}

// recordReconcileMetrics publishes the detection counts, which are the part of
// the sweep that is always on.
func recordReconcileMetrics(result *ReconcileResult) {
	m := getMetrics()
	if m == nil {
		return
	}
	for range result.DanglingRefs {
		m.RecordFileProcessed(reconcilePolicy, actionDanglingRef)
	}
	for range result.OrphanFiles {
		m.RecordFileProcessed(reconcilePolicy, actionOrphanFile)
	}
}
