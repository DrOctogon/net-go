// policy_guards_test.go - direct unit tests for the retention safety guards
// and the deletion helpers that actually remove files from disk. These are the
// highest-risk paths in the package (irreversible os.Remove), so they are
// tested directly against real files in t.TempDir().
package diskmanager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
)

// guardTestSpecies is the species used by the guard/deletion helper tests.
const guardTestSpecies = "strix_aluco"

// TestCheckLocked verifies the lock guard: a locked clip must always be
// reported as skippable, an unlocked clip never.
func TestCheckLocked(t *testing.T) {
	t.Parallel()

	locked := FileInfo{Path: "/clips/strix_aluco_80p_20210102T150405Z.wav", Species: guardTestSpecies, Locked: true}
	unlocked := FileInfo{Path: "/clips/strix_aluco_80p_20210102T150405Z.wav", Species: guardTestSpecies, Locked: false}

	assert.True(t, checkLocked(&locked), "Locked file must be skipped")
	assert.False(t, checkLocked(&unlocked), "Unlocked file must not be skipped")
}

// TestCheckMinClips verifies the minimum-clips-per-species guard, including
// the defensive branches for inconsistent count maps (which must refuse
// deletion rather than risk deleting below the minimum).
func TestCheckMinClips(t *testing.T) {
	t.Parallel()

	const subDir = "/clips/2025/01"
	file := &FileInfo{Path: filepath.Join(subDir, "strix_aluco_80p_20250102T150405Z.wav"), Species: guardTestSpecies}

	tests := []struct {
		name         string
		speciesCount map[string]map[string]int
		minClips     int
		wantDelete   bool
	}{
		{
			name:         "count above minimum allows deletion",
			speciesCount: map[string]map[string]int{guardTestSpecies: {subDir: 3}},
			minClips:     2,
			wantDelete:   true,
		},
		{
			name:         "count at minimum blocks deletion",
			speciesCount: map[string]map[string]int{guardTestSpecies: {subDir: 2}},
			minClips:     2,
			wantDelete:   false,
		},
		{
			name:         "count below minimum blocks deletion",
			speciesCount: map[string]map[string]int{guardTestSpecies: {subDir: 1}},
			minClips:     2,
			wantDelete:   false,
		},
		{
			name:         "missing subdirectory blocks deletion defensively",
			speciesCount: map[string]map[string]int{guardTestSpecies: {"/clips/2025/02": 10}},
			minClips:     0,
			wantDelete:   false,
		},
		{
			name:         "missing species blocks deletion defensively",
			speciesCount: map[string]map[string]int{"corvus_corax": {subDir: 10}},
			minClips:     0,
			wantDelete:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkMinClips(file, subDir, tc.speciesCount, tc.minClips, conf.RetentionPolicyUsage)
			assert.Equal(t, tc.wantDelete, got)
		})
	}
}

// TestDeleteAudioFile verifies the low-level deletion helper against real
// files: success removes the file, a missing file yields an error.
func TestDeleteAudioFile(t *testing.T) {
	t.Parallel()

	t.Run("removes existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		audioPath := createRetentionTestFile(t, dir, guardTestSpecies, 80, time.Now(), ".wav", 16)
		file := &FileInfo{Path: audioPath, Species: guardTestSpecies, Size: 16}

		require.NoError(t, deleteAudioFile(file, conf.RetentionPolicyAge))
		assert.NoFileExists(t, audioPath, "Audio file must be gone after deletion")
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		t.Parallel()
		file := &FileInfo{Path: filepath.Join(t.TempDir(), "missing.wav"), Species: guardTestSpecies, Size: 16}

		err := deleteAudioFile(file, conf.RetentionPolicyAge)
		require.Error(t, err, "Deleting a non-existent file must surface an error")
	})
}

// TestDeleteFileAndOptionalSpectrogram verifies the combined audio+spectrogram
// deletion path against real files, covering both KeepSpectrograms settings.
func TestDeleteFileAndOptionalSpectrogram(t *testing.T) {
	t.Parallel()

	t.Run("deletes audio and spectrogram", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		audioPath := createRetentionTestFile(t, dir, guardTestSpecies, 80, time.Now(), ".wav", 16)
		pngPath := createSpectrogramFor(t, audioPath)
		file := &FileInfo{Path: audioPath, Species: guardTestSpecies, Size: 16}

		require.NoError(t, deleteFileAndOptionalSpectrogram(file, "test", false, conf.RetentionPolicyAge))
		assert.NoFileExists(t, audioPath, "Audio file must be deleted")
		assert.NoFileExists(t, pngPath, "Spectrogram must be deleted when keepSpectrograms is false")
	})

	t.Run("keeps spectrogram when requested", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		audioPath := createRetentionTestFile(t, dir, guardTestSpecies, 80, time.Now(), ".wav", 16)
		pngPath := createSpectrogramFor(t, audioPath)
		file := &FileInfo{Path: audioPath, Species: guardTestSpecies, Size: 16}

		require.NoError(t, deleteFileAndOptionalSpectrogram(file, "test", true, conf.RetentionPolicyAge))
		assert.NoFileExists(t, audioPath, "Audio file must be deleted")
		assert.FileExists(t, pngPath, "Spectrogram must be kept when keepSpectrograms is true")
	})

	t.Run("missing spectrogram is not an error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		audioPath := createRetentionTestFile(t, dir, guardTestSpecies, 80, time.Now(), ".wav", 16)
		file := &FileInfo{Path: audioPath, Species: guardTestSpecies, Size: 16}

		require.NoError(t, deleteFileAndOptionalSpectrogram(file, "test", false, conf.RetentionPolicyAge))
		assert.NoFileExists(t, audioPath, "Audio file must be deleted")
	})

	t.Run("missing audio file returns error", func(t *testing.T) {
		t.Parallel()
		file := &FileInfo{Path: filepath.Join(t.TempDir(), "missing.wav"), Species: guardTestSpecies, Size: 16}

		err := deleteFileAndOptionalSpectrogram(file, "test", false, conf.RetentionPolicyAge)
		require.Error(t, err, "Deleting a non-existent audio file must surface an error")
	})
}

// TestHandleDeletionErrorInLoop verifies the deletion loop's error budget:
// isolated errors let the loop continue, but exceeding maxErrors must stop it
// with a terminal error.
func TestHandleDeletionErrorInLoop(t *testing.T) {
	t.Parallel()

	t.Run("continues below max errors", func(t *testing.T) {
		t.Parallel()
		errorCount := 0
		stop, loopErr := handleDeletionErrorInLoop("/clips/a.wav", os.ErrPermission, &errorCount, 10, conf.RetentionPolicyAge)
		assert.False(t, stop, "Loop must continue while under the error budget")
		require.NoError(t, loopErr)
		assert.Equal(t, 1, errorCount, "Error count must be incremented")
	})

	t.Run("stops once max errors exceeded", func(t *testing.T) {
		t.Parallel()
		errorCount := 10
		stop, loopErr := handleDeletionErrorInLoop("/clips/a.wav", os.ErrPermission, &errorCount, 10, conf.RetentionPolicyAge)
		assert.True(t, stop, "Loop must stop once the error budget is exhausted")
		require.Error(t, loopErr, "Stopping must surface a terminal error")
		assert.Equal(t, 11, errorCount)
	})
}

// TestCategorizeFilePath verifies path anonymization for metrics.
func TestCategorizeFilePath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "nested-path", categorizeFilePath("/clips/2025/file.wav"))
	assert.Equal(t, "nested-path", categorizeFilePath(`clips\2025\file.wav`))
	assert.Equal(t, "simple-filename", categorizeFilePath("file.wav"))
}

// TestShouldSkipUsageBasedCleanup verifies the early-exit guard that decides
// whether a usage-based run needs to scan files at all. Uses the disk-usage
// seams, so no t.Parallel().
func TestShouldSkipUsageBasedCleanup(t *testing.T) {
	baseDir := t.TempDir()

	t.Run("skips when usage below threshold", func(t *testing.T) {
		setFixedDiskUsage(t, 70, 1000)
		skip, utilization, err := ShouldSkipUsageBasedCleanup(&conf.RetentionSettings{MaxUsage: defaultMaxUsagePercent}, baseDir)
		require.NoError(t, err)
		assert.True(t, skip, "Cleanup should be skipped below threshold")
		assert.Equal(t, 70, utilization)
	})

	t.Run("proceeds when usage at or above threshold", func(t *testing.T) {
		setFixedDiskUsage(t, 90, 1000)
		skip, utilization, err := ShouldSkipUsageBasedCleanup(&conf.RetentionSettings{MaxUsage: defaultMaxUsagePercent}, baseDir)
		require.NoError(t, err)
		assert.False(t, skip, "Cleanup should proceed at or above threshold")
		assert.Equal(t, 90, utilization)
	})

	t.Run("empty MaxUsage falls back to default threshold", func(t *testing.T) {
		setFixedDiskUsage(t, 70, 1000)
		skip, utilization, err := ShouldSkipUsageBasedCleanup(&conf.RetentionSettings{MaxUsage: "  "}, baseDir)
		require.NoError(t, err)
		assert.True(t, skip, "70%% usage is below the default 80%% threshold")
		assert.Equal(t, 70, utilization)
	})

	t.Run("invalid MaxUsage returns error", func(t *testing.T) {
		setFixedDiskUsage(t, 90, 1000)
		skip, _, err := ShouldSkipUsageBasedCleanup(&conf.RetentionSettings{MaxUsage: "not-a-percentage"}, baseDir)
		require.Error(t, err, "Invalid threshold must surface a parse error")
		assert.False(t, skip)
	})
}
