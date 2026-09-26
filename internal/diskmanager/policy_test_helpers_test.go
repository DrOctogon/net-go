// policy_test_helpers_test.go - shared helpers for retention policy tests.
//
// These helpers exist so the retention scenario tests can exercise the REAL
// production entry points (AgeBasedCleanup / UsageBasedCleanup) end to end:
//   - applyRetentionSettings publishes global settings pointing the policies
//     at a t.TempDir(),
//   - overrideDiskUsage / setFixedDiskUsage / setDirBackedDiskUsage swap the
//     package-level disk-usage seams (see policy_common.go) for deterministic
//     numbers,
//   - createRetentionTestFile writes real audio files whose names the real
//     parser (parseFileInfo) understands.
//
// Tests using these helpers mutate package-level and global state and must
// NOT call t.Parallel().
package diskmanager

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/conf/conftest"
	mock_diskmanager "github.com/tphakala/voicewatch/internal/diskmanager/mocks"
)

// applyRetentionSettings publishes global test settings so the real cleanup
// entry points (which read conf.Setting()) operate on baseDir with the given
// retention configuration. Settings are cleared again at test cleanup.
func applyRetentionSettings(t *testing.T, baseDir string, retention *conf.RetentionSettings) {
	t.Helper()
	settings := conftest.GetTestSettings()
	settings.Realtime.Audio.Export.Path = baseDir
	settings.Realtime.Audio.Export.Retention = *retention
	conftest.SetTestSettings(settings)
	t.Cleanup(func() { conftest.SetTestSettings(nil) })
}

// overrideDiskUsage points the package-level disk-usage seams at test doubles
// for the duration of the test, restoring the real implementations afterwards.
func overrideDiskUsage(t *testing.T, usage func(string) (float64, error), detailed func(string) (DiskSpaceInfo, error)) {
	t.Helper()
	origUsage, origDetailed := getDiskUsage, getDetailedDiskUsage
	getDiskUsage = usage
	getDetailedDiskUsage = detailed
	t.Cleanup(func() {
		getDiskUsage = origUsage
		getDetailedDiskUsage = origDetailed
	})
}

// setFixedDiskUsage makes both disk-usage seams report a constant utilization
// percentage against the given total size.
func setFixedDiskUsage(t *testing.T, percent float64, totalBytes uint64) {
	t.Helper()
	overrideDiskUsage(t,
		func(string) (float64, error) { return percent, nil },
		func(string) (DiskSpaceInfo, error) {
			return DiskSpaceInfo{
				TotalBytes: totalBytes,
				UsedBytes:  uint64(percent) * totalBytes / 100,
			}, nil
		})
}

// setDirBackedDiskUsage models a disk where used bytes equal overheadBytes
// plus the combined size of the audio files currently under baseDir. As the
// real cleanup loop deletes files, the reported usage drops accordingly,
// exercising the loop's threshold stop condition with deterministic numbers.
func setDirBackedDiskUsage(t *testing.T, baseDir string, totalBytes, overheadBytes uint64) {
	t.Helper()
	detailed := func(string) (DiskSpaceInfo, error) {
		return DiskSpaceInfo{
			TotalBytes: totalBytes,
			UsedBytes:  overheadBytes + audioBytesUnder(baseDir),
		}, nil
	}
	overrideDiskUsage(t,
		func(path string) (float64, error) {
			info, err := detailed(path)
			if err != nil {
				return 0, err
			}
			return float64(info.UsedBytes) * 100 / float64(totalBytes), nil
		},
		detailed)
}

// audioBytesUnder sums the sizes of audio files (allowedFileTypes) under
// baseDir. Spectrogram PNGs are deliberately excluded so the disk-usage math
// in tests is independent of the KeepSpectrograms setting.
func audioBytesUnder(baseDir string) uint64 {
	var sum uint64
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // best-effort sum for tests
		}
		if contains(allowedFileTypes, strings.ToLower(filepath.Ext(path))) {
			sum += uint64(info.Size()) // #nosec G115 -- test file sizes are small and non-negative
		}
		return nil
	})
	return sum
}

// newCleanupMockDB builds a datastore mock suitable for full cleanup runs.
// lockedBasenames are returned from GetLockedNotesClipPaths so GetAudioFiles
// marks the matching files as locked. The post-deletion bookkeeping calls are
// optional (.Maybe()) since runs that delete nothing never make them.
func newCleanupMockDB(t *testing.T, lockedBasenames ...string) *mock_diskmanager.MockInterface {
	t.Helper()
	m := &mock_diskmanager.MockInterface{}
	m.On("GetLockedNotesClipPaths").Return(lockedBasenames, nil)
	m.On("ClearNoteClipPathsByNames", mock.Anything).Return(int64(0), nil).Maybe()
	m.On("ScrubSpeechDataByClipNames", mock.Anything).Return(int64(0), nil).Maybe()
	return m
}

// createRetentionTestFile writes a real audio file named so that the REAL
// cleanup path parses species, confidence and timestamp from the filename.
// IMPORTANT: parseFileInfo treats filename timestamps as LOCAL time despite
// the Z suffix, so the name is formatted from local time to make the parsed
// timestamp equal ts. The file content is sizeBytes long so usage-based tests
// can control the disk-usage math via real file sizes.
func createRetentionTestFile(t *testing.T, dir, species string, confidence int, ts time.Time, ext string, sizeBytes int) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750), "Failed to create directory: %s", dir)
	baseName := fmt.Sprintf("%s_%dp_%sZ", species, confidence, ts.Format("20060102T150405"))
	audioPath := filepath.Join(dir, baseName+ext)
	require.NoError(t, os.WriteFile(audioPath, bytes.Repeat([]byte("a"), sizeBytes), 0o644), //nolint:gosec // G306: test files don't require restrictive permissions
		"Failed to create audio file: %s", audioPath)
	return audioPath
}

// createSpectrogramFor writes the spectrogram PNG associated with audioPath.
func createSpectrogramFor(t *testing.T, audioPath string) string {
	t.Helper()
	pngPath := strings.TrimSuffix(audioPath, filepath.Ext(audioPath)) + ".png"
	require.NoError(t, os.WriteFile(pngPath, []byte("png"), 0o644), //nolint:gosec // G306: test files don't require restrictive permissions
		"Failed to create png file: %s", pngPath)
	return pngPath
}
