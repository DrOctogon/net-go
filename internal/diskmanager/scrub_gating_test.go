package diskmanager

import (
	"path/filepath"
	"testing"

	mock_diskmanager "github.com/tphakala/voicewatch/internal/diskmanager/mocks"
)

// TestClearDeletedClipPaths_ScrubEnabled verifies that when scrubSpeechData is
// true the speech data is scrubbed BEFORE the clip paths are cleared (the scrub
// matches notes by clip_name, which clearing would blank).
func TestClearDeletedClipPaths_ScrubEnabled(t *testing.T) {
	baseDir := t.TempDir()
	deleted := []string{filepath.Join(baseDir, "a.wav"), filepath.Join(baseDir, "b.wav")}
	want := []string{"a.wav", "b.wav"}

	mockDB := &mock_diskmanager.MockInterface{}
	scrub := mockDB.On("ScrubSpeechDataByClipNames", want).Return(int64(2), nil)
	// NotBefore enforces the ordering contract: clear must run after scrub.
	mockDB.On("ClearNoteClipPathsByNames", want).Return(int64(2), nil).NotBefore(scrub)

	clearDeletedClipPaths(mockDB, deleted, baseDir, "age", true)

	mockDB.AssertExpectations(t)
}

// TestClearDeletedClipPaths_ScrubDisabled verifies the scrub is skipped entirely
// when the operator has opted out, while clip paths are still cleared.
func TestClearDeletedClipPaths_ScrubDisabled(t *testing.T) {
	baseDir := t.TempDir()
	deleted := []string{filepath.Join(baseDir, "a.wav")}

	mockDB := &mock_diskmanager.MockInterface{}
	mockDB.On("ClearNoteClipPathsByNames", []string{"a.wav"}).Return(int64(1), nil)

	clearDeletedClipPaths(mockDB, deleted, baseDir, "age", false)

	mockDB.AssertExpectations(t)
	mockDB.AssertNotCalled(t, "ScrubSpeechDataByClipNames")
}
