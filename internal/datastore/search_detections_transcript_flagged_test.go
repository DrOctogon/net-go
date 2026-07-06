// search_detections_transcript_flagged_test.go: Tests for transcript-text and
// flagged-only filtering via SearchDetections (*SearchFilters path).
//
// Covers: case-insensitive substring match on transcript, literal treatment of
// LIKE wildcards (%, _), flagged-only / unflagged-only filtering, and that the
// total count returned by SearchDetections reflects the filters (not the full
// table).
package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchDetections_TranscriptFilter(t *testing.T) {
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

	t.Run("substring match returns correct detection", func(t *testing.T) {
		t.Parallel()
		results, total, err := store.SearchDetections(&SearchFilters{
			Transcript: "fire",
			PerPage:    50,
			Ctx:        t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "Fire", results[0].CommonName)
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		t.Parallel()
		results, total, err := store.SearchDetections(&SearchFilters{
			Transcript: "WESTERN",
			PerPage:    50,
			Ctx:        t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "Quiet", results[0].CommonName)
	})

	t.Run("non-matching term returns empty", func(t *testing.T) {
		t.Parallel()
		results, total, err := store.SearchDetections(&SearchFilters{
			Transcript: "no_such_phrase_xyz",
			PerPage:    50,
			Ctx:        t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, results)
	})

	t.Run("percent wildcard is treated literally", func(t *testing.T) {
		t.Parallel()
		// "50%" must match only the note with a literal "50%" in the transcript,
		// not every row (which an unescaped '%' wildcard would produce).
		results, total, err := store.SearchDetections(&SearchFilters{
			Transcript: "50%",
			PerPage:    50,
			Ctx:        t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "Percent", results[0].CommonName)
	})

	t.Run("empty transcript filter returns all detections", func(t *testing.T) {
		t.Parallel()
		results, total, err := store.SearchDetections(&SearchFilters{
			PerPage: 50,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, results, 3)
	})

	t.Run("total count reflects transcript filter", func(t *testing.T) {
		t.Parallel()
		// Use PerPage=1 so the result page is smaller than the unfiltered total,
		// confirming that the count query also applies the transcript filter.
		results, total, err := store.SearchDetections(&SearchFilters{
			Transcript: "quiet",
			PerPage:    1,
			Ctx:        t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total) // count query must apply the filter
		assert.Len(t, results, 1) // result query must also apply the filter
		assert.Equal(t, "Quiet", results[0].CommonName)
	})
}

func TestSearchDetections_FlaggedFilter(t *testing.T) {
	t.Parallel()

	store := createDatabase(t, createTestSettings(t))
	ds := store.(*SQLiteStore)

	flaggedNote := &Note{
		Date: "2026-06-25", Time: "10:00:00",
		CommonName: "Flagged", ScientificName: "A a",
		Flagged: true, KeywordsHit: "fire",
	}
	plainNote := &Note{
		Date: "2026-06-25", Time: "11:00:00",
		CommonName: "Plain", ScientificName: "B b",
	}
	require.NoError(t, ds.DB.Create(flaggedNote).Error)
	require.NoError(t, ds.DB.Create(plainNote).Error)

	t.Run("flagged=true returns only flagged detections", func(t *testing.T) {
		t.Parallel()
		flaggedTrue := true
		results, total, err := store.SearchDetections(&SearchFilters{
			Flagged: &flaggedTrue,
			PerPage: 50,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "Flagged", results[0].CommonName)
	})

	t.Run("flagged=false returns only unflagged detections", func(t *testing.T) {
		t.Parallel()
		flaggedFalse := false
		results, total, err := store.SearchDetections(&SearchFilters{
			Flagged: &flaggedFalse,
			PerPage: 50,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, results, 1)
		assert.Equal(t, "Plain", results[0].CommonName)
	})

	t.Run("nil flagged filter returns all detections", func(t *testing.T) {
		t.Parallel()
		results, total, err := store.SearchDetections(&SearchFilters{
			PerPage: 50,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, results, 2)
	})

	t.Run("total count reflects flagged filter", func(t *testing.T) {
		t.Parallel()
		// PerPage=1 so result page < unfiltered total; confirms count also filtered.
		flaggedTrue := true
		results, total, err := store.SearchDetections(&SearchFilters{
			Flagged: &flaggedTrue,
			PerPage: 1,
			Ctx:     t.Context(),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total) // count query must apply the flagged filter
		assert.Len(t, results, 1) // result query must also apply the flagged filter
		assert.Equal(t, "Flagged", results[0].CommonName)
	})
}
