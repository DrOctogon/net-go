// transcript_search_test.go: Tests for free-text transcript search via SearchNotesAdvanced.
//
// Covers: case-insensitive substring match, no-match, and literal treatment of
// LIKE wildcard characters (%, _) in the search term.
package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchNotesAdvanced_TranscriptFilter(t *testing.T) {
	t.Parallel()

	store := createDatabase(t, createTestSettings(t))
	ds := store.(*SQLiteStore)

	// Seed three notes with distinct transcripts.
	fire := &Note{
		Date: "2026-06-25", Time: "10:00:00",
		CommonName: "Fire", ScientificName: "A a",
		Transcript: "There is a fire nearby",
	}
	quiet := &Note{
		Date: "2026-06-25", Time: "11:00:00",
		CommonName: "Quiet", ScientificName: "B b",
		Transcript: "All quiet on the western front",
	}
	percent := &Note{
		Date: "2026-06-25", Time: "12:00:00",
		CommonName: "Percent", ScientificName: "C c",
		Transcript: "Battery at 50% charge",
	}
	require.NoError(t, ds.DB.Create(fire).Error)
	require.NoError(t, ds.DB.Create(quiet).Error)
	require.NoError(t, ds.DB.Create(percent).Error)

	t.Run("substring match returns correct note", func(t *testing.T) {
		t.Parallel()
		notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{
			Transcript: "fire",
			Limit:      50,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "Fire", notes[0].CommonName)
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		t.Parallel()
		notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{
			Transcript: "WESTERN",
			Limit:      50,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "Quiet", notes[0].CommonName)
	})

	t.Run("non-matching term returns empty", func(t *testing.T) {
		t.Parallel()
		notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{
			Transcript: "no_such_phrase_xyz",
			Limit:      50,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, notes)
	})

	t.Run("percent wildcard is treated literally", func(t *testing.T) {
		t.Parallel()
		// "50%" should match only the note with literal "50%" in transcript,
		// not every note (which a raw unescaped '%' wildcard would do).
		notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{
			Transcript: "50%",
			Limit:      50,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, notes, 1)
		assert.Equal(t, "Percent", notes[0].CommonName)
	})

	t.Run("empty transcript filter returns all notes", func(t *testing.T) {
		t.Parallel()
		notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{
			Limit: 50,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, notes, 3)
	})
}
