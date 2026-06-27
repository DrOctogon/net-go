// search_speaker_attributes_test.go: Tests for speaker-attribute filtering
// (gender / age band) via both search surfaces, plus persistence of the
// speaker columns and voice-print embedding through NoteFromResult.
package datastore

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/detection"
	"github.com/tphakala/voicewatch/internal/speaker"
)

func seedSpeakerNotes(t *testing.T) *SQLiteStore {
	t.Helper()
	store := createDatabase(t, createTestSettings(t))
	ds := store.(*SQLiteStore)

	notes := []*Note{
		{Date: "2026-06-25", Time: "10:00:00", CommonName: "MaleAdult", ScientificName: "A a",
			Gender: speaker.GenderMale, GenderConfidence: 0.9, AgeBand: speaker.AgeBandAdult, AgeConfidence: 0.8},
		{Date: "2026-06-25", Time: "11:00:00", CommonName: "FemaleChild", ScientificName: "B b",
			Gender: speaker.GenderFemale, GenderConfidence: 0.7, AgeBand: speaker.AgeBandChild, AgeConfidence: 0.6},
		{Date: "2026-06-25", Time: "12:00:00", CommonName: "NoAttrs", ScientificName: "C c"},
	}
	for _, n := range notes {
		require.NoError(t, ds.DB.Create(n).Error)
	}
	return ds
}

func TestSearchDetections_SpeakerAttributeFilters(t *testing.T) {
	t.Parallel()
	ds := seedSpeakerNotes(t)

	t.Run("gender filter (count reflects filter)", func(t *testing.T) {
		t.Parallel()
		results, total, err := ds.SearchDetections(&SearchFilters{
			Gender:  speaker.GenderFemale,
			PerPage: 1,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total) // count query applies the filter
		require.Len(t, results, 1)
		assert.Equal(t, "FemaleChild", results[0].CommonName)
	})

	t.Run("age band filter", func(t *testing.T) {
		t.Parallel()
		results, total, err := ds.SearchDetections(&SearchFilters{
			AgeBand: speaker.AgeBandAdult,
			PerPage: 50,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "MaleAdult", results[0].CommonName)
	})

	t.Run("combined gender + age band", func(t *testing.T) {
		t.Parallel()
		_, total, err := ds.SearchDetections(&SearchFilters{
			Gender:  speaker.GenderMale,
			AgeBand: speaker.AgeBandChild, // no row is male+child
			PerPage: 50,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, total)
	})

	t.Run("no filter returns all", func(t *testing.T) {
		t.Parallel()
		_, total, err := ds.SearchDetections(&SearchFilters{PerPage: 50, Ctx: t.Context()})
		require.NoError(t, err)
		assert.Equal(t, 3, total)
	})
}

func TestSearchNotesAdvanced_SpeakerAttributeFilters(t *testing.T) {
	t.Parallel()
	ds := seedSpeakerNotes(t)

	notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{
		Gender: speaker.GenderMale,
		Limit:  50,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, notes, 1)
	assert.Equal(t, "MaleAdult", notes[0].CommonName)

	notes, total, err = ds.SearchNotesAdvanced(&AdvancedSearchFilters{
		AgeBand: speaker.AgeBandChild,
		Limit:   50,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, notes, 1)
	assert.Equal(t, "FemaleChild", notes[0].CommonName)
}

func TestNoteFromResult_SpeakerAttributes(t *testing.T) {
	t.Parallel()
	emb := []float32{0.1, 0.2, 0.3}
	note := NoteFromResult(&detection.Result{
		Gender:              speaker.GenderFemale,
		GenderConfidence:    0.75,
		AgeBand:             speaker.AgeBandTeen,
		AgeConfidence:       0.65,
		SpeakerID:           "spk-7",
		VoicePrintEmbedding: emb,
	})
	assert.Equal(t, speaker.GenderFemale, note.Gender)
	assert.InDelta(t, 0.75, note.GenderConfidence, 1e-6)
	assert.Equal(t, speaker.AgeBandTeen, note.AgeBand)
	assert.InDelta(t, 0.65, note.AgeConfidence, 1e-6)
	assert.Equal(t, "spk-7", note.SpeakerID)
	assert.Equal(t, emb, note.VoicePrintEmbedding)
}

func TestVoicePrintEmbedding_RoundTrip(t *testing.T) {
	t.Parallel()
	store := createDatabase(t, createTestSettings(t))
	ds := store.(*SQLiteStore)

	emb := []float32{0.5, -0.25, 0.125}
	in := &Note{
		Date: "2026-06-25", Time: "09:00:00", CommonName: "Embedded", ScientificName: "D d",
		SpeakerID: "spk-1", VoicePrintEmbedding: emb,
	}
	require.NoError(t, ds.DB.Create(in).Error)

	got, err := store.Get(strconv.FormatUint(uint64(in.ID), 10))
	require.NoError(t, err)
	assert.Equal(t, "spk-1", got.SpeakerID)
	assert.Equal(t, emb, got.VoicePrintEmbedding, "serializer:json column round-trips []float32")
}
