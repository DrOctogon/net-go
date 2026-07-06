// keyword_flag_test.go: Tests for keyword-flag persistence and filtering.
//
// These tests exercise UpdateKeywordFlag (the additive flag setter) and the
// SearchNotesAdvanced flagged filter against a real SQLite-backed datastore.
package datastore

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateKeywordFlag_PersistsFlagAndKeywords(t *testing.T) {
	t.Parallel()

	store := createDatabase(t, createTestSettings(t))
	ds := store.(*SQLiteStore)
	repo := NewDetectionRepository(store, time.UTC)

	note := &Note{Date: "2026-06-25", Time: "10:00:00", CommonName: "Test", ScientificName: "Testus testus"}
	require.NoError(t, ds.DB.Create(note).Error)
	require.NotZero(t, note.ID)

	id := strconv.FormatUint(uint64(note.ID), 10)
	require.NoError(t, repo.UpdateKeywordFlag(t.Context(), id, true, "fire,help"))

	var got Note
	require.NoError(t, ds.DB.First(&got, note.ID).Error)
	assert.True(t, got.Flagged)
	assert.Equal(t, "fire,help", got.KeywordsHit)
	// Other columns must be untouched.
	assert.Equal(t, "Test", got.CommonName)
}

func TestSearchNotesAdvanced_FlaggedFilter(t *testing.T) {
	t.Parallel()

	store := createDatabase(t, createTestSettings(t))
	ds := store.(*SQLiteStore)

	flagged := &Note{Date: "2026-06-25", Time: "10:00:00", CommonName: "Flagged", ScientificName: "A a", Flagged: true, KeywordsHit: "fire"}
	plain := &Note{Date: "2026-06-25", Time: "11:00:00", CommonName: "Plain", ScientificName: "B b"}
	require.NoError(t, ds.DB.Create(flagged).Error)
	require.NoError(t, ds.DB.Create(plain).Error)

	flaggedTrue := true
	notes, total, err := ds.SearchNotesAdvanced(&AdvancedSearchFilters{Flagged: &flaggedTrue, Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, notes, 1)
	assert.Equal(t, "Flagged", notes[0].CommonName)

	flaggedFalse := false
	notes, total, err = ds.SearchNotesAdvanced(&AdvancedSearchFilters{Flagged: &flaggedFalse, Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, notes, 1)
	assert.Equal(t, "Plain", notes[0].CommonName)

	// Nil filter returns both.
	notes, total, err = ds.SearchNotesAdvanced(&AdvancedSearchFilters{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, notes, 2)
}
