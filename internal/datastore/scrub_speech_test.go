package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupScrubTestDB returns an in-memory legacy DataStore with the Note schema.
func setupScrubTestDB(t *testing.T) *DataStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Note{}))
	return &DataStore{DB: db}
}

// TestScrubSpeechDataByClipNames verifies that the retention scrub erases only
// the speech-derived and biometric-adjacent columns for the matched clips, while
// leaving detection identity/metadata and non-matching notes untouched.
func TestScrubSpeechDataByClipNames(t *testing.T) {
	t.Parallel()
	ds := setupScrubTestDB(t)

	// Two notes carry sensitive voice data; one has a clip that will be deleted,
	// the other must be left completely untouched.
	notes := []Note{
		{
			ID: 1, Date: "2026-07-06", Time: "10:00:00",
			ScientificName: "Homo sapiens", CommonName: "Human Voice", Confidence: 0.9,
			ClipName:   "2026/07/human_1.wav",
			Transcript: "the house is on fire", TranscriptLang: "en",
			Flagged: true, KeywordsHit: "fire",
			Gender: "male", GenderConfidence: 0.8,
			AgeBand: "adult", AgeConfidence: 0.7,
			SpeakerID: "spk_3", VoicePrintEmbedding: []float32{0.1, 0.2, 0.3},
		},
		{
			ID: 2, Date: "2026-07-06", Time: "11:00:00",
			ScientificName: "Homo sapiens", CommonName: "Human Voice", Confidence: 0.8,
			ClipName:   "2026/07/human_2.wav",
			Transcript: "keep this one", TranscriptLang: "en",
			SpeakerID: "spk_4", VoicePrintEmbedding: []float32{0.4, 0.5},
		},
	}
	for i := range notes {
		require.NoError(t, ds.DB.Create(&notes[i]).Error)
	}

	// Scrub only the first clip.
	scrubbed, err := ds.ScrubSpeechDataByClipNames([]string{"2026/07/human_1.wav"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), scrubbed, "exactly one note should be scrubbed")

	var got Note
	require.NoError(t, ds.DB.First(&got, 1).Error)
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
	assert.Equal(t, "Homo sapiens", got.ScientificName)
	assert.Equal(t, "Human Voice", got.CommonName)
	assert.InDelta(t, 0.9, got.Confidence, 0.001)
	assert.Equal(t, "2026/07/human_1.wav", got.ClipName)
	assert.True(t, got.Flagged, "keyword-match flag is retained; only the matched words are scrubbed")

	// The unrelated note is fully intact.
	var untouched Note
	require.NoError(t, ds.DB.First(&untouched, 2).Error)
	assert.Equal(t, "keep this one", untouched.Transcript)
	assert.Equal(t, "spk_4", untouched.SpeakerID)
	assert.Equal(t, []float32{0.4, 0.5}, untouched.VoicePrintEmbedding)
}

// TestScrubSpeechDataByClipNames_Empty is a no-op fast path.
func TestScrubSpeechDataByClipNames_Empty(t *testing.T) {
	t.Parallel()
	ds := setupScrubTestDB(t)
	scrubbed, err := ds.ScrubSpeechDataByClipNames(nil)
	require.NoError(t, err)
	assert.Zero(t, scrubbed)
}
