// speech_fields_test.go: Tests for speech-derived and speaker-attribute field
// persistence on the v2 store (phase 1 of the v2 speech port).
//
// These tests verify that the 10 speech/speaker columns round-trip through
// Save → Get (detectionToNote), that the backend-aware transcript/keyword-flag
// updates work against a real v2 SQLite database, and that the retention scrub
// erases exactly the same columns as the legacy implementation.
package v2only

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
)

const (
	speechTestDate    = "2026-07-06"
	speechTestTime    = "10:00:00"
	speechTestSciName = "Homo sapiens"
	speechTestComName = "Human Voice"
)

// speechTestNote returns a Note carrying every speech-derived and
// speaker-attribute field, plus the identity fields Save requires.
func speechTestNote(clipName string) *datastore.Note {
	return &datastore.Note{
		Date:                speechTestDate,
		Time:                speechTestTime,
		ScientificName:      speechTestSciName,
		CommonName:          speechTestComName,
		Confidence:          0.9,
		ClipName:            clipName,
		Transcript:          "the house is on fire",
		TranscriptLang:      "en",
		Flagged:             true,
		KeywordsHit:         "fire",
		Gender:              "male",
		GenderConfidence:    0.8,
		AgeBand:             "adult",
		AgeConfidence:       0.7,
		SpeakerID:           "spk_3",
		VoicePrintEmbedding: []float32{0.1, 0.2, 0.3},
	}
}

// TestSave_SpeechFieldsRoundTrip verifies that all 10 speech/speaker fields
// survive Save → Get on the v2 store.
func TestSave_SpeechFieldsRoundTrip(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()

	note := speechTestNote("2026/07/human_1.wav")
	require.NoError(t, ds.Save(note, nil))
	require.NotZero(t, note.ID)

	got, err := ds.Get(strconv.FormatUint(uint64(note.ID), 10))
	require.NoError(t, err)

	assert.Equal(t, "the house is on fire", got.Transcript)
	assert.Equal(t, "en", got.TranscriptLang)
	assert.True(t, got.Flagged)
	assert.Equal(t, "fire", got.KeywordsHit)
	assert.Equal(t, "male", got.Gender)
	assert.InDelta(t, 0.8, got.GenderConfidence, 0.001)
	assert.Equal(t, "adult", got.AgeBand)
	assert.InDelta(t, 0.7, got.AgeConfidence, 0.001)
	assert.Equal(t, "spk_3", got.SpeakerID)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got.VoicePrintEmbedding)
	// Identity fields still intact.
	assert.Equal(t, speechTestSciName, got.ScientificName)
	assert.Equal(t, "2026/07/human_1.wav", got.ClipName)
}

// TestSave_SpeechFieldsUnset verifies a VAD-only detection (no speech data)
// saves and reads back with zero values, not errors.
func TestSave_SpeechFieldsUnset(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()

	note := &datastore.Note{
		Date:           speechTestDate,
		Time:           "11:00:00",
		ScientificName: speechTestSciName,
		CommonName:     speechTestComName,
		Confidence:     0.5,
	}
	require.NoError(t, ds.Save(note, nil))

	got, err := ds.Get(strconv.FormatUint(uint64(note.ID), 10))
	require.NoError(t, err)
	assert.Empty(t, got.Transcript)
	assert.Empty(t, got.TranscriptLang)
	assert.False(t, got.Flagged)
	assert.Empty(t, got.KeywordsHit)
	assert.Empty(t, got.Gender)
	assert.Zero(t, got.GenderConfidence)
	assert.Empty(t, got.AgeBand)
	assert.Zero(t, got.AgeConfidence)
	assert.Empty(t, got.SpeakerID)
	assert.Nil(t, got.VoicePrintEmbedding)
}

// TestDetectionRepository_UpdateTranscript_V2 verifies the transcript update
// path works against a real v2 SQLite store (previously a hard error: the
// repository wrote to the nonexistent notes table).
func TestDetectionRepository_UpdateTranscript_V2(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	repo := datastore.NewDetectionRepository(ds, time.UTC)

	note := &datastore.Note{
		Date: speechTestDate, Time: speechTestTime,
		ScientificName: speechTestSciName, CommonName: speechTestComName, Confidence: 0.9,
	}
	require.NoError(t, ds.Save(note, nil))
	id := strconv.FormatUint(uint64(note.ID), 10)

	require.NoError(t, repo.UpdateTranscript(t.Context(), id, "hello world", "en"))

	got, err := ds.Get(id)
	require.NoError(t, err)
	assert.Equal(t, "hello world", got.Transcript)
	assert.Equal(t, "en", got.TranscriptLang)
	// Other columns untouched.
	assert.Equal(t, speechTestSciName, got.ScientificName)
	assert.InDelta(t, 0.9, got.Confidence, 0.001)
}

// TestDetectionRepository_UpdateKeywordFlag_V2 verifies the keyword-flag update
// path works against a real v2 SQLite store.
func TestDetectionRepository_UpdateKeywordFlag_V2(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	repo := datastore.NewDetectionRepository(ds, time.UTC)

	note := &datastore.Note{
		Date: speechTestDate, Time: speechTestTime,
		ScientificName: speechTestSciName, CommonName: speechTestComName, Confidence: 0.9,
	}
	require.NoError(t, ds.Save(note, nil))
	id := strconv.FormatUint(uint64(note.ID), 10)

	require.NoError(t, repo.UpdateKeywordFlag(t.Context(), id, true, "fire,help"))

	got, err := ds.Get(id)
	require.NoError(t, err)
	assert.True(t, got.Flagged)
	assert.Equal(t, "fire,help", got.KeywordsHit)
	assert.Equal(t, speechTestSciName, got.ScientificName)
}

// TestScrubSpeechDataByClipNames_V2 verifies the retention scrub erases only
// the speech-derived and biometric-adjacent columns for the matched clips,
// mirroring the legacy DataStore semantics (flag retained, identity preserved,
// non-matching detections untouched).
func TestScrubSpeechDataByClipNames_V2(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()

	scrubTarget := speechTestNote("2026/07/human_1.wav")
	require.NoError(t, ds.Save(scrubTarget, nil))

	keep := speechTestNote("2026/07/human_2.wav")
	keep.Transcript = "keep this one"
	keep.SpeakerID = "spk_4"
	keep.VoicePrintEmbedding = []float32{0.4, 0.5}
	require.NoError(t, ds.Save(keep, nil))

	scrubbed, err := ds.ScrubSpeechDataByClipNames([]string{"2026/07/human_1.wav"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), scrubbed, "exactly one detection should be scrubbed")

	got, err := ds.Get(strconv.FormatUint(uint64(scrubTarget.ID), 10))
	require.NoError(t, err)
	// Speech-derived + biometric columns cleared.
	assert.Empty(t, got.Transcript)
	assert.Empty(t, got.TranscriptLang)
	assert.Empty(t, got.KeywordsHit)
	assert.Empty(t, got.Gender)
	assert.Zero(t, got.GenderConfidence)
	assert.Empty(t, got.AgeBand)
	assert.Zero(t, got.AgeConfidence)
	assert.Empty(t, got.SpeakerID)
	assert.Nil(t, got.VoicePrintEmbedding)
	// Detection identity/metadata preserved — scrub is not a delete.
	assert.Equal(t, speechTestSciName, got.ScientificName)
	assert.InDelta(t, 0.9, got.Confidence, 0.001)
	assert.Equal(t, "2026/07/human_1.wav", got.ClipName)
	assert.True(t, got.Flagged, "keyword-match flag is retained; only the matched words are scrubbed")

	// The unrelated detection is fully intact.
	untouched, err := ds.Get(strconv.FormatUint(uint64(keep.ID), 10))
	require.NoError(t, err)
	assert.Equal(t, "keep this one", untouched.Transcript)
	assert.Equal(t, "spk_4", untouched.SpeakerID)
	assert.Equal(t, []float32{0.4, 0.5}, untouched.VoicePrintEmbedding)
}

// TestScrubSpeechDataByClipNames_V2_Empty is the no-op fast path.
func TestScrubSpeechDataByClipNames_V2_Empty(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()

	scrubbed, err := ds.ScrubSpeechDataByClipNames(nil)
	require.NoError(t, err)
	assert.Zero(t, scrubbed)
}
