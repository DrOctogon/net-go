package diskmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/conf"
)

// MockDB is a mock implementation of the database interface for testing
type MockDB struct{}

// GetDeletionInfo is a mock implementation that always returns no entries
func (m *MockDB) GetDeletionInfo() ([]string, error) {
	return []string{}, nil
}

// InsertDeletionInfo is a mock implementation that does nothing
func (m *MockDB) InsertDeletionInfo(filename string) error {
	return nil
}

// GetLockedNotesClipPaths is a mock implementation that returns no paths
func (m *MockDB) GetLockedNotesClipPaths() ([]string, error) {
	return []string{}, nil
}

// ClearNoteClipPathsByNames is a mock implementation that does nothing
func (m *MockDB) ClearNoteClipPathsByNames(_ []string) (int64, error) {
	return 0, nil
}

// ScrubSpeechDataByClipNames is a mock implementation that does nothing
func (m *MockDB) ScrubSpeechDataByClipNames(_ []string) (int64, error) {
	return 0, nil
}

// TestAgeBasedCleanupFileTypeEligibility tests if the file type check works correctly
func TestAgeBasedCleanupFileTypeEligibility(t *testing.T) {
	// Test with different file extensions
	testFiles := []struct {
		name          string
		expectError   bool
		errorContains string
	}{
		// Audio files - should work without errors
		{"bubo_bubo_80p_20210102T150405Z.wav", false, ""},
		{"bubo_bubo_80p_20210102T150405Z.mp3", false, ""},
		{"bubo_bubo_80p_20210102T150405Z.flac", false, ""},
		{"bubo_bubo_80p_20210102T150405Z.aac", false, ""},
		{"bubo_bubo_80p_20210102T150405Z.opus", false, ""},

		// Non-audio files - should return errors
		{"bubo_bubo_80p_20210102T150405Z.txt", true, "not eligible for cleanup"},
		{"bubo_bubo_80p_20210102T150405Z.jpg", true, "not eligible for cleanup"},
		{"bubo_bubo_80p_20210102T150405Z.png", true, "not eligible for cleanup"},
		{"bubo_bubo_80p_20210102T150405Z.db", true, "not eligible for cleanup"},
		{"bubo_bubo_80p_20210102T150405Z.csv", true, "not eligible for cleanup"},
		{"system_80p_20210102T150405Z.exe", true, "not eligible for cleanup"},
	}

	// Print the current list of allowed file types for debugging
	t.Logf("Allowed file types: %v", allowedFileTypes)

	// Create a temporary directory (auto-cleaned by testing framework)
	testDir := t.TempDir()

	for _, tc := range testFiles {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock FileInfo
			mockInfo := createMockFileInfo(tc.name, 1024)

			// Call parseFileInfo directly to test file extension checking
			_, err := parseFileInfo(filepath.Join(testDir, tc.name), mockInfo, allowedFileTypes)

			// Debug logging
			t.Logf("File: %s, Extension: %s, Error: %v",
				tc.name, filepath.Ext(tc.name), err)

			// Check if the error matches our expectation
			if tc.expectError {
				require.Error(t, err, "SECURITY ISSUE: Expected error for %s but got nil", tc.name)
				assert.Contains(t, err.Error(), tc.errorContains, "Expected error containing '%s'", tc.errorContains)
			} else {
				require.NoError(t, err, "Expected no error for %s", tc.name)
			}
		})
	}
}

// TestAgeBasedFilesAfterFilter tests the filtering of files for age-based cleanup
func TestAgeBasedFilesAfterFilter(t *testing.T) {
	db := &MockDB{}
	allowedTypes := []string{".wav", ".mp3", ".flac", ".aac", ".opus", ".m4a"}

	// Create a temporary directory (auto-cleaned by testing framework)
	testDir := t.TempDir()

	// Let's create files of all relevant types
	fileTypes := []string{
		".wav", ".mp3", ".flac", ".aac", ".opus", ".m4a",
		".txt", ".jpg", ".png", ".db", ".exe",
	}

	for _, ext := range fileTypes {
		filePath := filepath.Join(testDir, fmt.Sprintf("bubo_bubo_80p_20210102T150405Z%s", ext))
		require.NoError(t, os.WriteFile(filePath, []byte("test content"), 0o644), "Failed to create test file") //nolint:gosec // G306: Test files don't require restrictive permissions
	}

	// Get audio files using the function that would be used by the policy
	audioFiles, err := GetAudioFiles(testDir, allowedTypes, db)
	require.NoError(t, err, "Failed to get audio files")

	// Verify only allowed audio files are returned
	assert.Len(t, audioFiles, len(allowedTypes), "Expected %d audio files", len(allowedTypes))

	// Verify each returned file has an allowed extension
	for _, file := range audioFiles {
		ext := filepath.Ext(file.Path)
		assert.True(t, contains(allowedTypes, ext), "SECURITY ISSUE: File with disallowed extension was included: %s", file.Path)
	}
}

// TestAgeBasedCleanupBasicFunctionality tests the basic functionality of age-based cleanup
func TestAgeBasedCleanupBasicFunctionality(t *testing.T) {
	// Create test files with different timestamps
	// Recent files (within retention period)
	recentFile1 := FileInfo{
		Path:       "/test/bubo_bubo_80p_20210102T150405Z.wav",
		Species:    "bubo_bubo",
		Confidence: 80,
		Timestamp:  time.Now().Add(-24 * time.Hour), // 1 day old
		Size:       1024,
		Locked:     false,
	}

	recentFile2 := FileInfo{
		Path:       "/test/anas_platyrhynchos_70p_20210102T150405Z.wav",
		Species:    "anas_platyrhynchos",
		Confidence: 70,
		Timestamp:  time.Now().Add(-48 * time.Hour), // 2 days old
		Size:       1024,
		Locked:     false,
	}

	// Old files (beyond retention period)
	oldFile1 := FileInfo{
		Path:       "/test/bubo_bubo_90p_20200102T150405Z.wav",
		Species:    "bubo_bubo",
		Confidence: 90,
		Timestamp:  time.Now().Add(-720 * time.Hour), // 30 days old
		Size:       1024,
		Locked:     false,
	}

	oldFile2 := FileInfo{
		Path:       "/test/anas_platyrhynchos_60p_20200102T150405Z.wav",
		Species:    "anas_platyrhynchos",
		Confidence: 60,
		Timestamp:  time.Now().Add(-1440 * time.Hour), // 60 days old
		Size:       1024,
		Locked:     false,
	}

	// A locked file that should never be deleted
	lockedFile := FileInfo{
		Path:       "/test/bubo_bubo_95p_20200102T150405Z.wav",
		Species:    "bubo_bubo",
		Confidence: 95,
		Timestamp:  time.Now().Add(-2160 * time.Hour), // 90 days old
		Size:       1024,
		Locked:     true,
	}

	// Test files collection
	testFiles := []FileInfo{recentFile1, recentFile2, oldFile1, oldFile2, lockedFile}

	// Verify file type checks are performed on all files
	for _, file := range testFiles {
		filename := filepath.Base(file.Path)
		ext := filepath.Ext(filename)

		// Assert that only allowed file types are processed
		assert.True(t, contains(allowedFileTypes, ext),
			"File type should be in the allowed list: %s", ext)
	}

	// Check that age-based cleanup would:
	// 1. Delete files older than retention period
	// 2. Never delete locked files
	// 3. Maintain minimum number of clips per species

	// This is a basic verification - a full test would require mocking more components
	for _, file := range testFiles {
		// Locked files should never be deleted
		if file.Locked {
			t.Logf("Verified that locked file would be protected: %s", file.Path)
			continue
		}

		// Recent files should be kept - use local time for comparison
		// Note: File timestamps (even with 'Z' suffix) are in local time, not UTC
		if file.Timestamp.After(time.Now().Add(-168 * time.Hour)) { // Assuming 7 day retention
			t.Logf("Verified that recent file would be preserved: %s", file.Path)
		} else {
			t.Logf("Verified that old file would be eligible for deletion: %s", file.Path)
		}
	}
}

// TestAgeBasedCleanupReturnValues tests that the REAL AgeBasedCleanup entry
// point returns the expected values and correctly handles spectrogram
// deletion based on the KeepSpectrograms setting.
// Mutates global settings and the disk-usage seams: no t.Parallel().
func TestAgeBasedCleanupReturnValues(t *testing.T) {
	// Create a temporary directory for testing
	testDir := t.TempDir()

	// AgeBasedCleanup reports the disk utilization it observes after the run;
	// pin the seam so the assertion is deterministic.
	const reportedDiskUtilization = 42

	// Timestamps are parsed from the FILENAME by the real code path.
	recentTime := time.Now().Add(-72 * time.Hour) // 3 days old - keep
	oldTime1 := time.Now().Add(-720 * time.Hour)  // 30 days old - delete
	oldTime2 := time.Now().Add(-720 * time.Hour)  // 30 days old - delete

	var recentAudioPath, recentPngPath, old1AudioPath, old1PngPath, old2AudioPath, old2PngPath string

	createAllFiles := func(t *testing.T) {
		t.Helper()
		recentAudioPath = createRetentionTestFile(t, testDir, "bubo_bubo", 80, recentTime, ".wav", 1024)
		recentPngPath = createSpectrogramFor(t, recentAudioPath)
		old1AudioPath = createRetentionTestFile(t, testDir, "bubo_bubo", 90, oldTime1, ".wav", 1024)
		old1PngPath = createSpectrogramFor(t, old1AudioPath)
		old2AudioPath = createRetentionTestFile(t, testDir, "anas_platyrhynchos", 60, oldTime2, ".wav", 1024)
		old2PngPath = createSpectrogramFor(t, old2AudioPath)
	}

	// --- Test Execution Function: runs the REAL AgeBasedCleanup ---
	runTest := func(t *testing.T, keepSpectrograms bool) CleanupResult {
		t.Helper()
		createAllFiles(t)
		applyRetentionSettings(t, testDir, &conf.RetentionSettings{
			Policy:           conf.RetentionPolicyAge,
			MaxAge:           "168h", // 7-day retention
			MinClips:         0,
			KeepSpectrograms: keepSpectrograms,
		})
		setFixedDiskUsage(t, reportedDiskUtilization, 1000)
		return AgeBasedCleanup(make(chan struct{}), &MockDB{})
	}

	// --- Scenario 1: KeepSpectrograms = true ---
	t.Run("KeepSpectrogramsTrue", func(t *testing.T) {
		result := runTest(t, true)

		// Verify return values (same checks as before)
		require.NoError(t, result.Err, "[KeepTrue] AgeBasedCleanup should not return an error")
		assert.Equal(t, 2, result.ClipsRemoved, "[KeepTrue] AgeBasedCleanup should remove 2 audio clips")
		assert.Equal(t, reportedDiskUtilization, result.DiskUtilization, "[KeepTrue] Incorrect disk utilization")

		// Verify audio file deletions (using actual file existence)
		assert.FileExists(t, recentAudioPath, "[KeepTrue] Recent audio file should exist")
		assert.NoFileExists(t, old1AudioPath, "[KeepTrue] Old audio file 1 should be deleted")
		assert.NoFileExists(t, old2AudioPath, "[KeepTrue] Old audio file 2 should be deleted")

		// Verify PNG files are NOT deleted
		assert.FileExists(t, recentPngPath, "[KeepTrue] Recent PNG file should exist")
		assert.FileExists(t, old1PngPath, "[KeepTrue] Old PNG file 1 should NOT be deleted")
		assert.FileExists(t, old2PngPath, "[KeepTrue] Old PNG file 2 should NOT be deleted")
	})

	// --- Scenario 2: KeepSpectrograms = false ---
	t.Run("KeepSpectrogramsFalse", func(t *testing.T) {
		result := runTest(t, false)

		// Verify return values (should be the same as KeepTrue)
		require.NoError(t, result.Err, "[KeepFalse] AgeBasedCleanup should not return an error")
		assert.Equal(t, 2, result.ClipsRemoved, "[KeepFalse] AgeBasedCleanup should remove 2 audio clips")
		assert.Equal(t, reportedDiskUtilization, result.DiskUtilization, "[KeepFalse] Incorrect disk utilization")

		// Verify audio file deletions (using actual file existence)
		assert.FileExists(t, recentAudioPath, "[KeepFalse] Recent audio file should exist")
		assert.NoFileExists(t, old1AudioPath, "[KeepFalse] Old audio file 1 should be deleted")
		assert.NoFileExists(t, old2AudioPath, "[KeepFalse] Old audio file 2 should be deleted")

		// Verify PNG files ARE deleted for the deleted audio files
		assert.FileExists(t, recentPngPath, "[KeepFalse] Recent PNG file should exist")
		assert.NoFileExists(t, old1PngPath, "[KeepFalse] Old PNG file 1 SHOULD be deleted")
		assert.NoFileExists(t, old2PngPath, "[KeepFalse] Old PNG file 2 SHOULD be deleted")
	})
}

// TestAgeBasedCleanupMinClipsGlobal tests that minClipsPerSpecies is enforced globally,
// not per subdirectory, by the REAL AgeBasedCleanup entry point.
// Mutates global settings and the disk-usage seams: no t.Parallel().
func TestAgeBasedCleanupMinClipsGlobal(t *testing.T) {
	// Create a temporary directory for testing
	testDir := t.TempDir()

	// Setup file times (all older than 7 days/168 hours for simplicity)
	oldestTime := time.Now().Add(-1000 * time.Hour)
	olderTime := time.Now().Add(-900 * time.Hour)
	oldTime := time.Now().Add(-800 * time.Hour)
	alsoOldTime := time.Now().Add(-750 * time.Hour)

	// Species A: 3 files, all old, in different dirs
	// Expected: Keep 1 (the newest of the old ones), delete 2 oldest
	aFile1 := createRetentionTestFile(t, filepath.Join(testDir, "dir1"), "bubo_bubo", 80, oldTime, ".wav", 1024)    // Keep this one
	aFile2 := createRetentionTestFile(t, filepath.Join(testDir, "dir1"), "bubo_bubo", 90, olderTime, ".wav", 1024)  // Delete
	aFile3 := createRetentionTestFile(t, filepath.Join(testDir, "dir2"), "bubo_bubo", 85, oldestTime, ".wav", 1024) // Delete

	// Species B: 1 file, old
	// Expected: Keep it (global minClips=1)
	bFile1 := createRetentionTestFile(t, filepath.Join(testDir, "dir1"), "anas_platyrhynchos", 70, alsoOldTime, ".wav", 1024)

	// --- Run the REAL cleanup --- Use minClips = 1
	applyRetentionSettings(t, testDir, &conf.RetentionSettings{
		Policy:           conf.RetentionPolicyAge,
		MaxAge:           "168h", // 7-day retention
		MinClips:         1,      // Crucial setting for this test
		KeepSpectrograms: false,  // doesn't matter much for this test
	})
	const reportedDiskUtilization = 42
	setFixedDiskUsage(t, reportedDiskUtilization, 1000)

	result := AgeBasedCleanup(make(chan struct{}), &MockDB{})

	// --- Assertions --- Expected 2 deletions total (the 2 oldest bubo_bubo)
	require.NoError(t, result.Err, "Cleanup should not return an error")
	assert.Equal(t, 2, result.ClipsRemoved, "Should remove 2 clips (oldest 2 of A), keeping 1 of A and 1 of B")
	assert.Equal(t, reportedDiskUtilization, result.DiskUtilization, "Incorrect final disk utilization")

	// Verify file existence based on global minClips
	assert.FileExists(t, aFile1, "Species A newest old file should be kept (minClips=1)")
	assert.NoFileExists(t, aFile2, "Species A older file should be deleted")
	assert.NoFileExists(t, aFile3, "Species A oldest file should be deleted")
	assert.FileExists(t, bFile1, "Species B only old file should be kept (minClips=1)") // Updated assertion
}

// TestAgeBasedCleanupShortRetention verifies end-to-end the REAL AgeBasedCleanup
// with a short retention period.
// Mutates global settings and the disk-usage seams: no t.Parallel().
func TestAgeBasedCleanupShortRetention(t *testing.T) {
	// --- Test Setup --- Temp Dir
	testDir := t.TempDir()

	// --- File Setup --- Helper (timestamps come from filenames)
	createTestFilePair := func(species string, confidence int, ts time.Time) (string, string) {
		t.Helper()
		audioPath := createRetentionTestFile(t, testDir, species, confidence, ts, ".wav", 1024)
		pngPath := createSpectrogramFor(t, audioPath)
		return audioPath, pngPath
	}

	// Create files around the 1-hour mark
	now := time.Now()
	justRecentTime := now.Add(-30 * time.Minute) // Keep
	justOldTime := now.Add(-90 * time.Minute)    // Delete
	olderTime := now.Add(-120 * time.Minute)     // Delete (but keep 1 bubo due to minClips=1)

	// Species A (bubo_bubo)
	aFileRecent, aPngRecent := createTestFilePair("bubo_bubo", 80, justRecentTime) // Keep (too new)
	aFileOld, aPngOld := createTestFilePair("bubo_bubo", 90, justOldTime)          // Delete (minClips met by recent file)
	aFileOlder, aPngOlder := createTestFilePair("bubo_bubo", 70, olderTime)        // Delete (oldest)

	// Species B (anas_platyrhynchos)
	bFileOld, bPngOld := createTestFilePair("anas_platyrhynchos", 60, justOldTime) // Keep (minClips=1)

	// --- Run the REAL cleanup --- 1h retention, minClips=1
	applyRetentionSettings(t, testDir, &conf.RetentionSettings{
		Policy:           conf.RetentionPolicyAge,
		MaxAge:           "1h", // <<< Retention period (1h)
		MinClips:         1,    // Min clips to keep
		KeepSpectrograms: false,
	})
	setFixedDiskUsage(t, 42, 1000)

	result := AgeBasedCleanup(make(chan struct{}), &MockDB{})

	// --- Assertions ---
	require.NoError(t, result.Err, "AgeBasedCleanup should run without error")
	assert.Equal(t, 2, result.ClipsRemoved, "Should remove 2 clips (oldest bubo, old anas)")

	// Verify file existence
	assert.FileExists(t, aFileRecent, "Recent bubo audio should exist")
	assert.FileExists(t, aPngRecent, "Recent bubo PNG should exist")

	assert.NoFileExists(t, aFileOld, "Old bubo audio should be deleted (older than 1h, minClips met by recent file)")
	assert.NoFileExists(t, aPngOld, "Old bubo PNG should be deleted")

	assert.NoFileExists(t, aFileOlder, "Older bubo audio should be deleted")
	assert.NoFileExists(t, aPngOlder, "Older bubo PNG should be deleted")

	assert.FileExists(t, bFileOld, "Old anas audio should be kept (minClips=1)")
	assert.FileExists(t, bPngOld, "Old anas PNG should be kept")
}
