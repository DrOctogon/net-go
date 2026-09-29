// speech_filters_test.go: Tests for speech-derived search filters (transcript,
// flagged, gender, age band, speaker id) through the v2only adapter (phase 2b
// of the v2 speech port).
//
// Mirrors the legacy scenarios in internal/datastore/transcript_search_test.go
// and internal/datastore/search_speaker_attributes_test.go: case-insensitive
// substring transcript match with LIKE wildcards treated literally, exact-match
// speaker-attribute narrowing (NULL rows never match), and flagged filtering,
// via both SearchNotesAdvanced and SearchDetections.
package v2only

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
)

const (
	filterTestDate    = "2026-06-25"
	filterTestTime2   = "11:00:00"
	filterTestTime3   = "12:00:00"
	filterTestGenderM = "male"
	filterTestGenderF = "female"
	filterTestAdult   = "adult"
	filterTestChild   = "child"
	filterTestKeyword = "fire"
	filterTestLimit   = 50
)

// seedSpeechFilterNotes saves three detections with distinct speech fields:
//   - "A fire": transcript mentions fire, flagged, male adult, spk_1
//   - "B quiet": transcript mentions western front, female child, spk_2
//   - "C percent": transcript contains a literal "50%", no speaker attributes
func seedSpeechFilterNotes(t *testing.T, ds *Datastore) {
	t.Helper()
	notes := []*datastore.Note{
		{
			Date: filterTestDate, Time: speechTestTime,
			ScientificName: "A fire", CommonName: "Fire", Confidence: 0.9,
			Transcript: "There is a fire nearby", TranscriptLang: "en",
			Flagged: true, KeywordsHit: filterTestKeyword,
			Gender: filterTestGenderM, GenderConfidence: 0.9,
			AgeBand: filterTestAdult, AgeConfidence: 0.8,
			SpeakerID: "spk_1",
		},
		{
			Date: filterTestDate, Time: filterTestTime2,
			ScientificName: "B quiet", CommonName: "Quiet", Confidence: 0.8,
			Transcript: "All quiet on the western front", TranscriptLang: "en",
			Gender: filterTestGenderF, GenderConfidence: 0.7,
			AgeBand: filterTestChild, AgeConfidence: 0.6,
			SpeakerID: "spk_2",
		},
		{
			Date: filterTestDate, Time: filterTestTime3,
			ScientificName: "C percent", CommonName: "Percent", Confidence: 0.7,
			Transcript: "Battery at 50% charge", TranscriptLang: "en",
		},
	}
	for _, n := range notes {
		require.NoError(t, ds.Save(n, nil))
	}
}

func TestSearchNotesAdvanced_TranscriptFilter_V2(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	seedSpeechFilterNotes(t, ds)

	t.Run("substring match returns correct note", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Transcript: "fire", Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "There is a fire nearby", notes[0].Transcript)
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Transcript: "WESTERN", Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "All quiet on the western front", notes[0].Transcript)
	})

	t.Run("non-matching term returns empty", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Transcript: "no_such_phrase_xyz", Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, notes)
	})

	t.Run("percent wildcard is treated literally", func(t *testing.T) {
		// "50%" must match only the literal "50%" transcript, not every row
		// (which a raw unescaped '%' wildcard would).
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Transcript: "50%", Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "Battery at 50% charge", notes[0].Transcript)
	})

	t.Run("underscore wildcard is treated literally", func(t *testing.T) {
		// "w_stern" would match "western" via an unescaped '_' wildcard.
		_, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Transcript: "w_stern", Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
	})

	t.Run("empty transcript filter returns all notes", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, notes, 3)
	})
}

func TestSearchNotesAdvanced_SpeakerAttributeFilters_V2(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	seedSpeechFilterNotes(t, ds)

	t.Run("gender filter", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Gender: filterTestGenderM, Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "spk_1", notes[0].SpeakerID)
	})

	t.Run("age band filter", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			AgeBand: filterTestChild, Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "spk_2", notes[0].SpeakerID)
	})

	t.Run("speaker id filter", func(t *testing.T) {
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			SpeakerID: "spk_2", Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, filterTestGenderF, notes[0].Gender)
	})

	t.Run("combined gender + age band narrows to zero", func(t *testing.T) {
		_, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Gender: filterTestGenderM, AgeBand: filterTestChild, // no male child row
			Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
	})

	t.Run("flagged true filter", func(t *testing.T) {
		flagged := true
		notes, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Flagged: &flagged, Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "fire", notes[0].KeywordsHit)
	})

	t.Run("flagged false filter", func(t *testing.T) {
		flagged := false
		_, total, err := ds.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
			Flagged: &flagged, Limit: filterTestLimit,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
	})
}

func TestSearchDetections_SpeechFilters_V2(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	seedSpeechFilterNotes(t, ds)

	t.Run("gender filter with count reflecting filter", func(t *testing.T) {
		results, total, err := ds.SearchDetections(&datastore.SearchFilters{
			Gender:  filterTestGenderF,
			PerPage: 1,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total) // count query applies the filter
		require.Len(t, results, 1)
		assert.Equal(t, "B quiet", results[0].ScientificName)
	})

	t.Run("transcript filter", func(t *testing.T) {
		results, total, err := ds.SearchDetections(&datastore.SearchFilters{
			Transcript: "FIRE", // case-insensitive
			PerPage:    filterTestLimit,
			Ctx:        t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "A fire", results[0].ScientificName)
	})

	t.Run("speaker id filter", func(t *testing.T) {
		results, total, err := ds.SearchDetections(&datastore.SearchFilters{
			SpeakerID: "spk_1",
			PerPage:   filterTestLimit,
			Ctx:       t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "A fire", results[0].ScientificName)
	})

	t.Run("flagged filter", func(t *testing.T) {
		flagged := true
		_, total, err := ds.SearchDetections(&datastore.SearchFilters{
			Flagged: &flagged,
			PerPage: filterTestLimit,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
	})

	t.Run("age band filter", func(t *testing.T) {
		_, total, err := ds.SearchDetections(&datastore.SearchFilters{
			AgeBand: filterTestAdult,
			PerPage: filterTestLimit,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
	})

	t.Run("no filter returns all", func(t *testing.T) {
		_, total, err := ds.SearchDetections(&datastore.SearchFilters{
			PerPage: filterTestLimit,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 3, total)
	})
}
