// policy_reconcile_test.go - tests for the dangling-clip reconciliation sweep.
//
// The sweep is the only retention code path that can mutate the database
// without deleting a file first, and (under an explicit opt-in) delete files
// that no database row points at. Both drift directions are therefore tested
// against real files in t.TempDir() through the real entry point
// (ReconcileClips), with the safety guards exercised directly.
//
// These tests publish global settings via applyRetentionSettings and must NOT
// call t.Parallel() (see policy_test_helpers_test.go).
package diskmanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
	mock_diskmanager "github.com/tphakala/voicewatch/internal/diskmanager/mocks"
)

// reconcileTestSpecies is the species used by the reconciliation tests.
const reconcileTestSpecies = "homo_sapiens"

// reconcileStore pairs the shared diskmanager mock (locked clips, clearing,
// scrubbing) with a deterministic paged clip-name source, so the sweep's real
// paging loop is exercised without teaching the shared mock about paging.
type reconcileStore struct {
	*mock_diskmanager.MockInterface
	clipNames []string
	pages     int
}

// ListClipNames serves clipNames one page at a time, mirroring the offset
// paging contract the datastore implementations provide.
func (s *reconcileStore) ListClipNames(_ context.Context, limit, offset int) ([]string, error) {
	s.pages++
	if offset >= len(s.clipNames) {
		return nil, nil
	}
	return s.clipNames[offset:min(offset+limit, len(s.clipNames))], nil
}

// newReconcileStore builds a store whose database reports the given clip
// references and the given locked clip basenames. No expectation is registered
// for ClearNoteClipPathsByNames or ScrubSpeechDataByClipNames, so a test that
// expects no mutation gets a hard failure if the sweep mutates anything.
func newReconcileStore(t *testing.T, lockedBasenames []string, clipNames ...string) *reconcileStore {
	t.Helper()
	m := &mock_diskmanager.MockInterface{}
	m.On("GetLockedNotesClipPaths").Return(lockedBasenames, nil).Maybe()
	return &reconcileStore{MockInterface: m, clipNames: clipNames}
}

// applyReconcileSettings points the sweep at baseDir with the given orphan
// deletion opt-in.
func applyReconcileSettings(t *testing.T, baseDir string, deleteOrphans bool) {
	t.Helper()
	applyRetentionSettings(t, baseDir, &conf.RetentionSettings{
		Policy:            conf.RetentionPolicyAge,
		MaxAge:            "30d",
		KeepSpectrograms:  true,
		DeleteOrphanClips: deleteOrphans,
	})
}

// relName returns the clip_name form (slash-separated, relative to baseDir) of
// an absolute clip path, matching how the datastores store it.
func relName(t *testing.T, baseDir, absPath string) string {
	t.Helper()
	rel, err := filepath.Rel(baseDir, absPath)
	require.NoError(t, err)
	return filepath.ToSlash(rel)
}

// TestReconcileClipsDanglingReference verifies that a reference to a clip whose
// file is definitively gone is detected and cleared, while a healthy reference
// is left untouched.
func TestReconcileClipsDanglingReference(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, false)

	present := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80, time.Now(), ".wav", 16)
	presentName := relName(t, baseDir, present)
	const danglingName = "homo_sapiens_70p_20250101T101010Z.wav"

	db := newReconcileStore(t, nil, presentName, danglingName)
	db.On("ClearNoteClipPathsByNames", []string{danglingName}).Return(int64(1), nil).Once()

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Empty(t, result.GuardTripped, "No guard should trip for one missing clip out of two")
	assert.Equal(t, 2, result.ClipRefsChecked)
	assert.Equal(t, 1, result.DanglingRefs, "Only the reference with no file may be reported dangling")
	assert.Equal(t, int64(1), result.RefsCleared)
	assert.Zero(t, result.UnreadableRefs)
	assert.Zero(t, result.OrphanFiles, "The present clip is referenced, so it is not an orphan")
	assert.FileExists(t, present, "A referenced clip must never be touched by the sweep")
	db.AssertExpectations(t)
}

// TestReconcileClipsStatErrorIsSkipped verifies the core safety distinction:
// only fs.ErrNotExist justifies clearing a reference. A reference whose parent
// path component is not a directory produces a non-ErrNotExist stat error
// (ENOTDIR) and must be left completely alone.
func TestReconcileClipsStatErrorIsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows maps a non-directory path component to a not-found error")
	}

	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, false)

	// Healthy, referenced clips keep both drift fractions below the guard
	// bounds so this test isolates the stat-error classification.
	names := make([]string, 0, 4)
	for i := range 3 {
		path := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80+i, time.Now().Add(-time.Duration(i)*time.Minute), ".wav", 16)
		names = append(names, relName(t, baseDir, path))
	}

	// "blocked" is a regular file, so statting through it fails with ENOTDIR.
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "blocked"), []byte("x"), 0o600))

	const blockedName = "blocked/homo_sapiens_60p_20250101T101010Z.wav"
	db := newReconcileStore(t, nil, append(names, blockedName)...)

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Empty(t, result.GuardTripped, "No guard may trip: this run has one unreadable reference and nothing else")
	assert.Zero(t, result.DanglingRefs, "A non-ErrNotExist stat error must never count as dangling")
	assert.Equal(t, 1, result.UnreadableRefs, "The unreadable reference must be reported as skipped")
	assert.Zero(t, result.RefsCleared)
	db.AssertNotCalled(t, "ClearNoteClipPathsByNames", mock.Anything)
}

// TestReconcileClipsOrphanNotDeletedByDefault verifies the non-destructive
// default: an orphan file is detected and reported but stays on disk.
func TestReconcileClipsOrphanNotDeletedByDefault(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, false)

	referenced := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80, time.Now(), ".wav", 16)
	orphan := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 55, time.Now().Add(-time.Hour), ".wav", 32)

	db := newReconcileStore(t, nil, relName(t, baseDir, referenced))

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Empty(t, result.GuardTripped)
	assert.Equal(t, 1, result.OrphanFiles, "The unreferenced file must be reported as an orphan")
	assert.Equal(t, int64(32), result.OrphanBytes)
	assert.Zero(t, result.OrphansDeleted, "Orphan deletion must be off by default")
	assert.FileExists(t, orphan, "Orphan files must survive the default sweep")
	assert.FileExists(t, referenced)
}

// TestReconcileClipsOrphanDeletedUnderOptIn verifies that the explicit opt-in,
// and only the opt-in, removes orphan files.
func TestReconcileClipsOrphanDeletedUnderOptIn(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, true)

	referenced := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80, time.Now(), ".wav", 16)
	orphan := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 55, time.Now().Add(-time.Hour), ".wav", 32)

	db := newReconcileStore(t, nil, relName(t, baseDir, referenced))

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Equal(t, 1, result.OrphanFiles)
	assert.Equal(t, 1, result.OrphansDeleted)
	assert.Equal(t, int64(32), result.BytesFreed)
	assert.NoFileExists(t, orphan, "The opt-in must delete the orphan file")
	assert.FileExists(t, referenced, "A referenced clip must never be deleted as an orphan")
}

// TestReconcileClipsLockedOrphanIsNeverDeleted verifies that a locked clip is
// never treated as a deletable orphan, even under the opt-in and even when its
// clip reference is missing from the reference listing.
func TestReconcileClipsLockedOrphanIsNeverDeleted(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, true)

	locked := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80, time.Now(), ".wav", 16)

	// The clip is locked but its reference is absent from the listing, which is
	// the worst case: the lock is the only thing protecting it.
	db := newReconcileStore(t, []string{filepath.Base(locked)})

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Zero(t, result.OrphanFiles, "A locked clip must not be reported as an orphan")
	assert.Zero(t, result.OrphansDeleted)
	assert.FileExists(t, locked, "Locked clips must survive orphan deletion")
}

// TestReconcileClipsMassMissingGuard verifies the catastrophic-case guard: when
// most referenced clips are missing (a failed mount, a database restored
// without its clips), the sweep must report and refuse to act.
func TestReconcileClipsMassMissingGuard(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, true)

	// One clip present, four references missing => 80% missing.
	present := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80, time.Now(), ".wav", 16)
	names := make([]string, 0, 5)
	names = append(names, relName(t, baseDir, present))
	for i := range 4 {
		names = append(names, fmt.Sprintf("homo_sapiens_6%dp_2025010%dT101010Z.wav", i, i+1))
	}

	db := newReconcileStore(t, nil, names...)

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Equal(t, guardMassMissing, result.GuardTripped, "The sanity guard must refuse to act")
	assert.Equal(t, 4, result.DanglingRefs, "Detection must still report what it found")
	assert.Zero(t, result.RefsCleared, "No reference may be cleared once the guard trips")
	assert.Zero(t, result.OrphansDeleted, "No file may be deleted once the guard trips")
	assert.FileExists(t, present)
	db.AssertNotCalled(t, "ClearNoteClipPathsByNames", mock.Anything)
}

// TestReconcileClipsEmptyDirGuard verifies that an empty (or unmounted) clip
// directory is never read as "every clip is missing".
func TestReconcileClipsEmptyDirGuard(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, true)

	db := newReconcileStore(t, nil,
		"homo_sapiens_80p_20250101T101010Z.wav",
		"homo_sapiens_81p_20250101T101011Z.wav")

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Equal(t, guardEmptyClipDir, result.GuardTripped,
		"An empty clip directory must trip the unmounted-volume guard")
	assert.Equal(t, 2, result.DanglingRefs, "Detection must still report what it found")
	assert.Zero(t, result.RefsCleared)
	db.AssertNotCalled(t, "ClearNoteClipPathsByNames", mock.Anything)
}

// TestReconcileClipsContextCancellation verifies the sweep stops when its
// context is cancelled and never mutates anything on the way out.
func TestReconcileClipsContextCancellation(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, true)

	createRetentionTestFile(t, baseDir, reconcileTestSpecies, 80, time.Now(), ".wav", 16)

	db := newReconcileStore(t, nil, "homo_sapiens_70p_20250101T101010Z.wav")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result := ReconcileClips(ctx, db)

	require.Error(t, result.Err, "A cancelled sweep must surface the context error")
	require.ErrorIs(t, result.Err, context.Canceled)
	assert.Zero(t, result.RefsCleared)
	db.AssertNotCalled(t, "ClearNoteClipPathsByNames", mock.Anything)
}

// TestReconcileClipsPagesThroughReferences verifies the sweep pages the clip
// reference query instead of loading the whole table in one go, and terminates.
func TestReconcileClipsPagesThroughReferences(t *testing.T) {
	baseDir := t.TempDir()
	applyReconcileSettings(t, baseDir, false)

	// More references than one page, all backed by real files so nothing is
	// dangling and no guard trips.
	total := clipRefPageSize + 7
	names := make([]string, 0, total)
	start := time.Now().Add(-time.Duration(total) * time.Minute)
	for i := range total {
		path := createRetentionTestFile(t, baseDir, reconcileTestSpecies, 50, start.Add(time.Duration(i)*time.Minute), ".wav", 8)
		names = append(names, relName(t, baseDir, path))
	}

	db := newReconcileStore(t, nil, names...)

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Equal(t, total, result.ClipRefsChecked, "Every reference must be visited across pages")
	assert.Greater(t, db.pages, 1, "More references than one page must require more than one query")
	assert.Zero(t, result.DanglingRefs)
	assert.Zero(t, result.OrphanFiles)
}

// TestReconcileGuard covers the guard decision table directly, including the
// mass-orphan case that indicates the stored clip names do not match the disk
// layout at all and therefore invalidate the whole comparison.
func TestReconcileGuard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		refsChecked   int
		danglingRefs  int
		orphanFiles   int
		diskFileCount int
		want          string
	}{
		{name: "nothing to do", want: ""},
		{name: "healthy install", refsChecked: 100, danglingRefs: 3, orphanFiles: 2, diskFileCount: 99, want: ""},
		{name: "empty clip dir with refs", refsChecked: 10, danglingRefs: 10, diskFileCount: 0, want: guardEmptyClipDir},
		{name: "half missing is tolerated", refsChecked: 10, danglingRefs: 5, diskFileCount: 5, want: ""},
		{name: "majority missing refuses", refsChecked: 10, danglingRefs: 6, diskFileCount: 4, want: guardMassMissing},
		{name: "majority orphan refuses", refsChecked: 10, danglingRefs: 0, orphanFiles: 9, diskFileCount: 10, want: guardMassOrphan},
		{name: "no refs but files present", orphanFiles: 4, diskFileCount: 4, want: guardMassOrphan},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := &ReconcileResult{
				ClipRefsChecked: tc.refsChecked,
				DanglingRefs:    tc.danglingRefs,
				OrphanFiles:     tc.orphanFiles,
			}
			assert.Equal(t, tc.want, reconcileGuard(result, tc.diskFileCount))
		})
	}
}

// TestReconcileClipsUnconfiguredPath verifies the sweep is a no-op when no
// export path is configured, rather than walking the process working directory.
func TestReconcileClipsUnconfiguredPath(t *testing.T) {
	applyReconcileSettings(t, "   ", false)

	db := newReconcileStore(t, nil)

	result := ReconcileClips(t.Context(), db)

	require.NoError(t, result.Err)
	assert.Zero(t, result.ClipRefsChecked)
	assert.Zero(t, db.pages, "An unconfigured export path must not query the database")
}
