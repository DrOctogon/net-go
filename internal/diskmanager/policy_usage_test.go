package diskmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/conf"
)

// MockFileInfo implements os.FileInfo for testing
type MockFileInfo struct {
	FileName    string
	FileSize    int64
	FileMode    os.FileMode
	FileModTime time.Time
	FileIsDir   bool
	FileSys     any
}

func (m *MockFileInfo) Name() string       { return m.FileName }
func (m *MockFileInfo) Size() int64        { return m.FileSize }
func (m *MockFileInfo) Mode() os.FileMode  { return m.FileMode }
func (m *MockFileInfo) ModTime() time.Time { return m.FileModTime }
func (m *MockFileInfo) IsDir() bool        { return m.FileIsDir }
func (m *MockFileInfo) Sys() any           { return m.FileSys }

// Helper function to create a mock FileInfo
func createMockFileInfo(filename string, size int64) os.FileInfo {
	return &MockFileInfo{
		FileName:    filename,
		FileSize:    size,
		FileMode:    0o644,
		FileModTime: time.Now(),
		FileIsDir:   false,
	}
}

// Helper function to parse time string
func parseTime(timeStr string) time.Time {
	t, _ := time.Parse("20060102T150405Z", timeStr)
	return t
}

// TestFileTypesEligibleForDeletion tests which file types are eligible for deletion
func TestFileTypesEligibleForDeletion(t *testing.T) {
	// Test cases with various file extensions
	testCases := []struct {
		filename            string
		extension           string
		eligibleForDeletion bool
		description         string
	}{
		// Allowed file types (should be eligible for deletion)
		{"bubo_bubo_80p_20210102T150405Z.wav", ".wav", true, "WAV files should be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.mp3", ".mp3", true, "MP3 files should be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.flac", ".flac", true, "FLAC files should be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.aac", ".aac", true, "AAC files should be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.opus", ".opus", true, "OPUS files should be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.m4a", ".m4a", true, "M4A files should be eligible for deletion"},

		// Disallowed file types (should not be eligible for deletion)
		{"bubo_bubo_80p_20210102T150405Z.txt", ".txt", false, "TXT files should not be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.jpg", ".jpg", false, "JPG files should not be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.png", ".png", false, "PNG files should not be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.db", ".db", false, "DB files should not be eligible for deletion"},
		{"bubo_bubo_80p_20210102T150405Z.csv", ".csv", false, "CSV files should not be eligible for deletion"},
		{"system_80p_20210102T150405Z.exe", ".exe", false, "EXE files should not be eligible for deletion"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			mockInfo := createMockFileInfo(tc.filename, 1024)
			fileInfo, err := parseFileInfo("/test/"+tc.filename, mockInfo, allowedFileTypes)

			if tc.eligibleForDeletion {
				require.NoError(t, err, "File should be eligible for deletion: %s", tc.description)
				assert.Equal(t, "bubo_bubo", fileInfo.Species, "Species should be correctly parsed")
				assert.Equal(t, 80, fileInfo.Confidence, "Confidence should be correctly parsed")

				// Check that the timestamp was parsed correctly
				// IMPORTANT: Timestamps from filenames (with 'Z') are now parsed as local time
				// so we need to compare against the local time equivalent.
				expectedTimeLocal, err := time.ParseInLocation("20060102T150405", "20210102T150405", time.Local)
				require.NoError(t, err, "Failed to parse expected local time for comparison")
				assert.Equal(t, expectedTimeLocal, fileInfo.Timestamp, "Timestamp should be correctly parsed")
			} else {
				// For disallowed files, we must ensure they would be rejected from deletion
				// We'll fail the test if they would be processed (which indicates a security issue)

				// Check if this file extension is in the allowedFileTypes list
				isAllowedExt := contains(allowedFileTypes, tc.extension)

				// Check if parseFileInfo returned an error
				hasError := (err != nil)

				// If the file has a disallowed extension but would be processed for deletion,
				// fail the test with a security warning
				assert.True(t, isAllowedExt || hasError,
					"SECURITY ISSUE: %s file would be processed for deletion but should be protected: %s",
					tc.extension, tc.description)

				// If the function returned an error, validate it's the right kind of error
				if hasError {
					assert.Contains(t, err.Error(), "file type",
						"Error should indicate file type issue")
					assert.Contains(t, err.Error(), "not eligible",
						"Error message should indicate file is not eligible for cleanup")
				}
			}
		})
	}
}

// TestParseFileInfoWithDifferentExtensions tests that parseFileInfo correctly handles different file extensions
func TestParseFileInfoWithDifferentExtensions(t *testing.T) {
	// Test cases for each allowed file type (.wav, .flac, .aac, .opus, .mp3)
	testCases := []struct {
		filename      string
		expectedExt   string
		shouldSucceed bool
	}{
		{"bubo_bubo_80p_20210102T150405Z.wav", ".wav", true},
		{"bubo_bubo_80p_20210102T150405Z.mp3", ".mp3", true},
		{"bubo_bubo_80p_20210102T150405Z.flac", ".flac", true},
		{"bubo_bubo_80p_20210102T150405Z.aac", ".aac", true},
		{"bubo_bubo_80p_20210102T150405Z.opus", ".opus", true},
		{"bubo_bubo_80p_20210102T150405Z.m4a", ".m4a", true},
		{"bubo_bubo_80p_20210102T150405Z.txt", ".txt", false}, // Unsupported extension
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			mockInfo := createMockFileInfo(tc.filename, 1024)
			fileInfo, err := parseFileInfo("/test/"+tc.filename, mockInfo, allowedFileTypes)

			if tc.shouldSucceed {
				require.NoError(t, err, "Should parse successfully")
				assert.Equal(t, "bubo_bubo", fileInfo.Species)
				assert.Equal(t, 80, fileInfo.Confidence)

				// Check that the timestamp was parsed correctly
				// IMPORTANT: Timestamps from filenames (with 'Z') are now parsed as local time
				// so we need to compare against the local time equivalent.
				expectedTimeLocal, err := time.ParseInLocation("20060102T150405", "20210102T150405", time.Local)
				require.NoError(t, err, "Failed to parse expected local time for comparison")
				assert.Equal(t, expectedTimeLocal, fileInfo.Timestamp, "Timestamp should be correctly parsed")
			} else {
				assert.Error(t, err, "Should return an error")
			}
		})
	}
}

// TestParseFileInfoMp3Extension specifically tests the MP3 extension bug
func TestParseFileInfoMp3Extension(t *testing.T) {
	// This test specifically targets the bug in the error message
	mockInfo := createMockFileInfo("bubo_bubo_80p_20250130T184446Z.mp3", 1024)

	fileInfo, err := parseFileInfo("/test/bubo_bubo_80p_20250130T184446Z.mp3", mockInfo, allowedFileTypes)

	// The bug would cause an error here because it only trims .wav extension
	require.NoError(t, err, "Should parse MP3 files correctly")
	assert.Equal(t, "bubo_bubo", fileInfo.Species)
	assert.Equal(t, 80, fileInfo.Confidence)

	// Check that the timestamp was parsed correctly
	// IMPORTANT: Timestamps from filenames (with 'Z') are now parsed as local time
	// so we need to compare against the local time equivalent.
	expectedTimeLocal, err := time.ParseInLocation("20060102T150405", "20250130T184446", time.Local)
	require.NoError(t, err, "Failed to parse expected local time for comparison")
	assert.Equal(t, expectedTimeLocal, fileInfo.Timestamp, "Timestamp should be correctly parsed")
}

// TestParseFileInfoProductionFormat tests file names as they actually appear in production
func TestParseFileInfoProductionFormat(t *testing.T) {
	// Test cases with real-world file names from production
	testCases := []struct {
		filename      string
		expectedSpec  string
		expectedConf  int
		expectedTime  string
		shouldSucceed bool
		description   string
	}{
		{
			// Standard production format with underscored species name
			"vulpes_vulpes_92p_20250223T195727Z.flac",
			"vulpes_vulpes",
			92,
			"20250223T195727Z",
			true,
			"Standard production file with multi-part species name",
		},
		{
			// PNG thumbnail with size suffix - this should fail due to file type
			"vulpes_vulpes_96p_20250223T073356Z_400px.png",
			"",
			0,
			"",
			false,
			"PNG file with size suffix should fail due to file type",
		},
		{
			// Three-part species name
			"genus_species_subspecies_99p_20250222T043210Z.flac",
			"genus_species_subspecies",
			99,
			"20250222T043210Z",
			true,
			"File with three-part species name",
		},
		{
			// Audio file with thumbnail size suffix - with our fix, this should now parse correctly
			"vulpes_vulpes_96p_20250223T073356Z_400px.flac",
			"vulpes_vulpes",
			96,
			"20250223T073356Z",
			true,
			"Audio file with size suffix should now parse correctly with the fix",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			mockInfo := createMockFileInfo(tc.filename, 1024)
			fileInfo, err := parseFileInfo("/test/"+tc.filename, mockInfo, allowedFileTypes)

			if tc.shouldSucceed {
				require.NoError(t, err, "Should parse successfully: "+tc.description)
				assert.Equal(t, tc.expectedSpec, fileInfo.Species, "Species should be correctly parsed")
				assert.Equal(t, tc.expectedConf, fileInfo.Confidence, "Confidence should be correctly parsed")

				// Check that the timestamp was parsed correctly
				// IMPORTANT: Timestamps from filenames (with 'Z') are now parsed as local time
				// so we need to compare against the local time equivalent.

				// First, parse the timestamp string without the Z suffix
				timestampStrLocal := strings.TrimSuffix(tc.expectedTime, "Z")

				// Parse it using the local timezone
				expectedTimeLocal, err := time.ParseInLocation("20060102T150405", timestampStrLocal, time.Local)
				require.NoError(t, err, "Failed to parse expected local time for comparison")
				assert.Equal(t, expectedTimeLocal, fileInfo.Timestamp, "Timestamp should be correctly parsed")
			} else {
				assert.Error(t, err, "Should return an error: "+tc.description)
				if err != nil {
					t.Logf("Error as expected: %v", err)
				}
			}
		})
	}
}

// TestSortFiles tests the sortFiles function
func TestSortFiles(t *testing.T) {
	// Create a set of files with different timestamps, species counts, and confidence levels
	// Note: parseTime helper now parses timestamps as local time due to changes in parseFileInfo
	files := []FileInfo{
		{Path: "/base/dir1/bubo_bubo_80p_20210102T150405Z.wav", Species: "bubo_bubo", Confidence: 80, Timestamp: parseTime("20210102T150405Z")},
		{Path: "/base/dir1/bubo_bubo_90p_20210103T150405Z.wav", Species: "bubo_bubo", Confidence: 90, Timestamp: parseTime("20210103T150405Z")},
		{Path: "/base/dir1/anas_platyrhynchos_70p_20210101T150405Z.wav", Species: "anas_platyrhynchos", Confidence: 70, Timestamp: parseTime("20210101T150405Z")},
	}

	// Sort the files
	speciesCount := buildSpeciesSubDirCountMap(files)
	sortFilesForUsage(files, speciesCount)

	// Verify sorting order (oldest first)
	assert.Equal(t, "anas_platyrhynchos", files[0].Species, "Anas platyrhynchos should be first (oldest)")
	assert.Equal(t, "bubo_bubo", files[1].Species, "Bubo bubo (oldest) should be second")
	assert.Equal(t, "bubo_bubo", files[2].Species, "Bubo bubo (newest) should be third")

	// Verify the count map is correct. buildSpeciesSubDirCountMap keys by
	// filepath.Dir(file.Path), which uses the OS separator (backslashes on
	// Windows), so derive the expected sub-directory key the same way rather
	// than hardcoding a forward-slash literal.
	subDir := filepath.Dir(files[0].Path)
	assert.Equal(t, 1, speciesCount["anas_platyrhynchos"][subDir])
	assert.Equal(t, 2, speciesCount["bubo_bubo"][subDir])
}

// ----- Tests for Usage-Based Cleanup -----
//
// These scenarios run the REAL UsageBasedCleanup entry point against real
// files in t.TempDir(). Disk usage comes from the seams in policy_common.go
// (see policy_test_helpers_test.go): "dir-backed" usage drops as the real
// deletion loop removes files, so the threshold stop condition and the
// safety guards (checkLocked, checkMinClips) are exercised end to end.
// All of these tests mutate global settings and the seams: no t.Parallel().

// usageRetention builds the retention settings used by the usage scenarios.
func usageRetention(minClips int, keepSpectrograms bool) *conf.RetentionSettings {
	return &conf.RetentionSettings{
		Policy:           conf.RetentionPolicyUsage,
		MaxUsage:         defaultMaxUsagePercent,
		MinClips:         minClips,
		KeepSpectrograms: keepSpectrograms,
	}
}

// usageTS returns a deterministic local-time timestamp in January 2021 for
// usage-policy test files (the usage policy ignores file age for eligibility,
// but ordering is oldest-first).
func usageTS(day int) time.Time {
	return time.Date(2021, 1, day, 15, 4, 5, 0, time.Local)
}

// TestUsageBasedCleanupTriggered tests that the real cleanup deletes files when
// usage exceeds the threshold and that spectrogram deletion respects the
// KeepSpectrograms setting.
func TestUsageBasedCleanupTriggered(t *testing.T) {
	runScenario := func(t *testing.T, keepSpectrograms bool) {
		t.Helper()
		tempDir := t.TempDir()

		// Two bubo_bubo clips and one anas clip, each with a spectrogram.
		audioPath1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 10)
		pngPath1 := createSpectrogramFor(t, audioPath1)
		audioPath2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 85, usageTS(2), ".wav", 10)
		pngPath2 := createSpectrogramFor(t, audioPath2)
		audioPath3 := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 70, usageTS(3), ".wav", 10)
		pngPath3 := createSpectrogramFor(t, audioPath3)

		applyRetentionSettings(t, tempDir, usageRetention(1, keepSpectrograms))
		// 890 + 3*10 = 920 of 1000 bytes = 92% usage: above the 80% threshold,
		// and it stays above it for the whole run, so the loop only stops once
		// minClipsPerSpecies blocks every remaining file.
		setDirBackedDiskUsage(t, tempDir, 1000, 890)

		result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t))
		require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")

		// Only the oldest bubo_bubo clip is deletable: the newer bubo clip and
		// the single anas clip are protected by minClipsPerSpecies=1.
		assert.Equal(t, 1, result.ClipsRemoved, "Should remove exactly the oldest bubo_bubo clip")
		assert.Equal(t, 91, result.DiskUtilization, "Final utilization should reflect the deleted bytes")
		assert.NoFileExists(t, audioPath1, "Oldest bubo audio should be deleted")
		assert.FileExists(t, audioPath2, "Newest bubo audio should be kept (minClips=1)")
		assert.FileExists(t, audioPath3, "Only anas audio should be kept (minClips=1)")

		if keepSpectrograms {
			assert.FileExists(t, pngPath1, "PNG of deleted clip should survive with KeepSpectrograms=true")
		} else {
			assert.NoFileExists(t, pngPath1, "PNG of deleted clip should be deleted with KeepSpectrograms=false")
		}
		assert.FileExists(t, pngPath2, "PNG of kept clip should exist")
		assert.FileExists(t, pngPath3, "PNG of kept clip should exist")
	}

	t.Run("KeepSpectrogramsTrue", func(t *testing.T) { runScenario(t, true) })
	t.Run("KeepSpectrogramsFalse", func(t *testing.T) { runScenario(t, false) })
}

// TestUsageBasedCleanupNoTriggerBelowThreshold tests that the real cleanup does
// nothing when usage is below the threshold.
func TestUsageBasedCleanupNoTriggerBelowThreshold(t *testing.T) {
	tempDir := t.TempDir()
	audioPath := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 10)

	applyRetentionSettings(t, tempDir, usageRetention(1, false))
	setFixedDiskUsage(t, 70, 1000) // 70% usage is below the 80% threshold

	result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t))

	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")
	assert.Equal(t, 0, result.ClipsRemoved, "No files should be deleted when usage is below threshold")
	assert.Equal(t, 70, result.DiskUtilization, "Reported utilization should match the (untouched) disk usage")
	assert.FileExists(t, audioPath, "File must survive a below-threshold run")
}

// TestUsageBasedCleanupWithAllFileTypes tests that all supported file types are
// cleaned up by the real cleanup while minClipsPerSpecies is respected.
func TestUsageBasedCleanupWithAllFileTypes(t *testing.T) {
	tempDir := t.TempDir()

	buboOld1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 10)
	buboOld2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 85, usageTS(2), ".mp3", 10)
	anas := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 70, usageTS(3), ".flac", 10)
	erithacus := createRetentionTestFile(t, tempDir, "erithacus_rubecula", 60, usageTS(4), ".aac", 10)
	passer := createRetentionTestFile(t, tempDir, "passer_domesticus", 90, usageTS(5), ".opus", 10)
	turdus := createRetentionTestFile(t, tempDir, "turdus_migratorius", 95, usageTS(5), ".m4a", 10)
	buboOld3 := createRetentionTestFile(t, tempDir, "bubo_bubo", 75, usageTS(6), ".wav", 10)
	buboKeep1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 65, usageTS(7), ".mp3", 10)
	buboKeep2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 95, usageTS(8), ".flac", 10)

	applyRetentionSettings(t, tempDir, usageRetention(2, true)) // Keep at least 2 clips per species
	// 880 + 9*10 = 970 of 1000 = 97%; usage never drops below 80% during the
	// run, so every eligible file gets deleted and only the guards stop it.
	setDirBackedDiskUsage(t, tempDir, 1000, 880)

	result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t))
	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")

	// Of the 5 bubo_bubo files exactly 3 (the oldest) are deletable; every
	// other species has a single clip, protected by minClipsPerSpecies=2.
	assert.Equal(t, 3, result.ClipsRemoved, "Should delete exactly 3 bubo_bubo clips")
	assert.NoFileExists(t, buboOld1, "Oldest bubo (.wav) should be deleted")
	assert.NoFileExists(t, buboOld2, "Second-oldest bubo (.mp3) should be deleted")
	assert.NoFileExists(t, buboOld3, "Third-oldest bubo (.wav) should be deleted")
	assert.FileExists(t, buboKeep1, "Should keep at least 2 bubo_bubo files (minClipsPerSpecies)")
	assert.FileExists(t, buboKeep2, "Should keep at least 2 bubo_bubo files (minClipsPerSpecies)")
	assert.FileExists(t, anas, "Single anas clip should be protected by minClips")
	assert.FileExists(t, erithacus, "Single erithacus clip should be protected by minClips")
	assert.FileExists(t, passer, "Single passer clip should be protected by minClips")
	assert.FileExists(t, turdus, "Single turdus clip should be protected by minClips")
}

// TestUsageBasedCleanupRespectLockedFiles verifies that the real cleanup never
// deletes locked files (lock information flows from the datastore through
// GetAudioFiles into the deletion loop's checkLocked guard).
func TestUsageBasedCleanupRespectLockedFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Multiple bubo files - the two oldest will be deleted
	bubo1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 10)
	bubo2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 82, usageTS(2), ".wav", 10)
	bubo3 := createRetentionTestFile(t, tempDir, "bubo_bubo", 85, usageTS(3), ".wav", 10)

	// Locked file - should never be deleted
	lockedFilePath := createRetentionTestFile(t, tempDir, "erithacus_rubecula", 80, usageTS(1), ".wav", 10)

	// Multiple anas files - the two oldest will be deleted
	anas1 := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 70, usageTS(2), ".mp3", 10)
	anas2 := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 72, usageTS(3), ".mp3", 10)
	anas3 := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 75, usageTS(4), ".mp3", 10)

	applyRetentionSettings(t, tempDir, usageRetention(1, true)) // Keep at least 1 clip per species
	// 900 + 7*10 = 970 of 1000 = 97%; stays above threshold all run.
	setDirBackedDiskUsage(t, tempDir, 1000, 900)

	result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t, filepath.Base(lockedFilePath)))
	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")

	// 2 bubo + 2 anas deletable; the locked erithacus is untouchable even
	// though minClips alone would also protect it.
	assert.Equal(t, 4, result.ClipsRemoved, "Should delete the 2 oldest bubo and 2 oldest anas clips")
	assert.FileExists(t, lockedFilePath, "Locked file should not be deleted")
	assert.NoFileExists(t, bubo1, "Oldest bubo should be deleted")
	assert.NoFileExists(t, bubo2, "Second-oldest bubo should be deleted")
	assert.FileExists(t, bubo3, "Newest bubo should be kept (minClips=1)")
	assert.NoFileExists(t, anas1, "Oldest anas should be deleted")
	assert.NoFileExists(t, anas2, "Second-oldest anas should be deleted")
	assert.FileExists(t, anas3, "Newest anas should be kept (minClips=1)")
}

// TestUsageBasedCleanupWithYearMonthFolders tests the real cleanup with the
// production-like year/month folder structure: minClipsPerSpecies is enforced
// per species per subdirectory.
func TestUsageBasedCleanupWithYearMonthFolders(t *testing.T) {
	tempDir := t.TempDir()

	monthDir1 := filepath.Join(tempDir, "2024", "12")
	monthDir2 := filepath.Join(tempDir, "2025", "01")
	monthDir3 := filepath.Join(tempDir, "2025", "02")

	localTS := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 15, 4, 5, 0, time.Local)
	}

	// Files in 2024/12
	buboDec1 := createRetentionTestFile(t, monthDir1, "bubo_bubo", 80, localTS(2024, 12, 15), ".wav", 10)
	buboDec2 := createRetentionTestFile(t, monthDir1, "bubo_bubo", 85, localTS(2024, 12, 20), ".mp3", 10)

	// Files in 2025/01
	erithacusJan1 := createRetentionTestFile(t, monthDir2, "erithacus_rubecula", 70, localTS(2025, 1, 5), ".flac", 10)
	erithacusJan2 := createRetentionTestFile(t, monthDir2, "erithacus_rubecula", 75, localTS(2025, 1, 10), ".flac", 10)

	// Files in 2025/02
	anasFeb1 := createRetentionTestFile(t, monthDir3, "anas_platyrhynchos", 60, localTS(2025, 2, 5), ".aac", 10)
	anasFeb2 := createRetentionTestFile(t, monthDir3, "anas_platyrhynchos", 65, localTS(2025, 2, 10), ".aac", 10)
	buboFeb := createRetentionTestFile(t, monthDir3, "bubo_bubo", 90, localTS(2025, 2, 15), ".opus", 10)

	applyRetentionSettings(t, tempDir, usageRetention(1, true)) // Keep at least 1 clip per species per directory
	// 920 + 7*10 = 990 of 1000 = 99%; stays above threshold all run.
	setDirBackedDiskUsage(t, tempDir, 1000, 920)

	result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t))
	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")

	// One deletion per (species, month dir) that has two clips; the lone
	// bubo in 2025/02 is protected even though bubo has other clips elsewhere.
	assert.Equal(t, 3, result.ClipsRemoved, "Should delete the older clip of each per-directory pair")
	assert.NoFileExists(t, buboDec1, "Older 2024/12 bubo should be deleted")
	assert.FileExists(t, buboDec2, "Should keep 1 bubo in 2024/12 (per-directory minClips)")
	assert.NoFileExists(t, erithacusJan1, "Older 2025/01 erithacus should be deleted")
	assert.FileExists(t, erithacusJan2, "Should keep 1 erithacus in 2025/01 (per-directory minClips)")
	assert.NoFileExists(t, anasFeb1, "Older 2025/02 anas should be deleted")
	assert.FileExists(t, anasFeb2, "Should keep 1 anas in 2025/02 (per-directory minClips)")
	assert.FileExists(t, buboFeb, "Lone 2025/02 bubo should be kept (per-directory minClips)")
}

// TestUsageBasedCleanupReturnValues tests that the real UsageBasedCleanup
// returns the expected values and stops deleting once usage drops below the
// threshold.
func TestUsageBasedCleanupReturnValues(t *testing.T) {
	tempDir := t.TempDir()

	// Each file is 60 bytes so each deletion drops usage by 6 percentage points.
	filePath1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 60)
	filePath2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 85, usageTS(2), ".wav", 60)
	filePath3 := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 70, usageTS(3), ".wav", 60)

	applyRetentionSettings(t, tempDir, usageRetention(0, true)) // minClips=0: only the threshold stops the loop
	// 720 + 3*60 = 900 of 1000 = 90%. After two deletions usage is 78%,
	// below the 80% threshold, so the loop must stop before the third file.
	setDirBackedDiskUsage(t, tempDir, 1000, 720)

	result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t))

	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")
	assert.Equal(t, 2, result.ClipsRemoved, "UsageBasedCleanup should remove 2 clips")
	assert.Equal(t, 78, result.DiskUtilization, "UsageBasedCleanup should report 78% disk utilization")

	// The two oldest files were deleted; the third survives because disk usage
	// dropped below the threshold after two deletions.
	assert.NoFileExists(t, filePath1, "File should have been deleted: %s", filePath1)
	assert.NoFileExists(t, filePath2, "File should have been deleted: %s", filePath2)
	assert.FileExists(t, filePath3, "File should not have been deleted: %s", filePath3)
}

// TestUsageBasedCleanupBelowThreshold tests that no files are deleted by the
// real cleanup when disk usage is below the threshold.
func TestUsageBasedCleanupBelowThreshold(t *testing.T) {
	tempDir := t.TempDir()

	filePath1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 60)
	filePath2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 85, usageTS(2), ".wav", 60)
	filePath3 := createRetentionTestFile(t, tempDir, "anas_platyrhynchos", 70, usageTS(3), ".wav", 60)

	applyRetentionSettings(t, tempDir, usageRetention(0, true))
	setFixedDiskUsage(t, 70, 1000) // 70% usage is below the 80% threshold

	result := UsageBasedCleanup(make(chan struct{}), newCleanupMockDB(t))

	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")
	assert.Equal(t, 0, result.ClipsRemoved, "UsageBasedCleanup should not remove any clips")
	assert.Equal(t, 70, result.DiskUtilization, "UsageBasedCleanup should report 70% disk utilization")

	assert.FileExists(t, filePath1, "File should not have been deleted: %s", filePath1)
	assert.FileExists(t, filePath2, "File should not have been deleted: %s", filePath2)
	assert.FileExists(t, filePath3, "File should not have been deleted: %s", filePath3)
}

// TestUsageBasedCleanupLockedFiles tests that locked files are not deleted by
// the real cleanup even when nothing else (minClips=0) would protect them.
func TestUsageBasedCleanupLockedFiles(t *testing.T) {
	tempDir := t.TempDir()

	bubo1 := createRetentionTestFile(t, tempDir, "bubo_bubo", 80, usageTS(1), ".wav", 50)
	bubo2 := createRetentionTestFile(t, tempDir, "bubo_bubo", 85, usageTS(2), ".wav", 50)
	lockedFilePath := createRetentionTestFile(t, tempDir, "erithacus_rubecula", 80, usageTS(1), ".wav", 50)

	applyRetentionSettings(t, tempDir, usageRetention(0, true)) // minClips=0: the lock is the only protection
	// 800 + 3*50 = 950 of 1000 = 95%; stays above threshold all run, so the
	// locked file is visited and only the lock saves it.
	setDirBackedDiskUsage(t, tempDir, 1000, 800)

	mockDB := newCleanupMockDB(t, filepath.Base(lockedFilePath))
	result := UsageBasedCleanup(make(chan struct{}), mockDB)

	mockDB.AssertExpectations(t)

	require.NoError(t, result.Err, "UsageBasedCleanup should not return an error")
	assert.Equal(t, 2, result.ClipsRemoved, "UsageBasedCleanup should remove the 2 non-locked clips")
	assert.Equal(t, 85, result.DiskUtilization, "UsageBasedCleanup should report 85% disk utilization")

	assert.NoFileExists(t, bubo1, "Non-locked file should have been deleted")
	assert.NoFileExists(t, bubo2, "Non-locked file should have been deleted")
	assert.FileExists(t, lockedFilePath, "Locked file should not have been deleted")
}
