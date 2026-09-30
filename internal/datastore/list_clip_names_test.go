// list_clip_names_test.go: Tests for the paged clip-reference listing used by
// the disk manager's reconciliation sweep, against a real SQLite database.
package datastore

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/diskmanager"
)

// The reconciliation sweep reaches the datastore through a runtime type
// assertion to diskmanager.ReconcileStore, which fails silently (reconciliation
// is skipped) if this datastore stops satisfying it. Assert it at compile time
// so a rename here is a build failure, not a quietly disabled sweep.
var _ diskmanager.ReconcileStore = (*DataStore)(nil)

// Fixed identity fields for the clip-reference listing fixtures. Hoisted so
// this file adds no new repeated string literals to the package.
const (
	clipNameTestDate    = "2026-07-06"
	clipNameTestTime    = "10:00:00"
	clipNameTestSciName = "Homo sapiens"
	clipNameTestComName = "Human Voice"
)

// clipNameTestNote returns a minimal note carrying the given clip reference.
func clipNameTestNote(id uint, clipName string) *Note {
	return &Note{
		ID: id, Date: clipNameTestDate, Time: clipNameTestTime,
		ScientificName: clipNameTestSciName, CommonName: clipNameTestComName, Confidence: 0.9,
		ClipName: clipName,
	}
}

// TestListClipNames verifies that only non-empty clip references are returned,
// in a stable order, and that limit/offset page through them without gaps or
// repeats.
func TestListClipNames(t *testing.T) {
	t.Parallel()
	ds := setupScrubTestDB(t)
	ctx := t.Context()

	// Five notes with clips, interleaved with two that have none. The empty
	// ones must never appear: a detection with no clip is not drift.
	want := make([]string, 0, 5)
	id := uint(1)
	for i := range 5 {
		clip := fmt.Sprintf("2026/07/human_%d.wav", i)
		require.NoError(t, ds.DB.Create(clipNameTestNote(id, clip)).Error)
		want = append(want, clip)
		id++
		require.NoError(t, ds.DB.Create(clipNameTestNote(id, "")).Error)
		id++
	}

	t.Run("returns only non-empty references", func(t *testing.T) {
		got, err := ds.ListClipNames(ctx, 100, 0)
		require.NoError(t, err)
		assert.Equal(t, want, got, "Notes without a clip must not be listed")
	})

	t.Run("pages without gaps or repeats", func(t *testing.T) {
		var got []string
		for offset := 0; ; offset += 2 {
			page, err := ds.ListClipNames(ctx, 2, offset)
			require.NoError(t, err)
			if len(page) == 0 {
				break
			}
			assert.LessOrEqual(t, len(page), 2, "A page must never exceed the limit")
			got = append(got, page...)
		}
		assert.Equal(t, want, got)
	})

	t.Run("offset past the end returns empty", func(t *testing.T) {
		got, err := ds.ListClipNames(ctx, 10, 500)
		require.NoError(t, err)
		assert.Empty(t, got, "An exhausted listing must terminate the sweep's paging loop")
	})
}

// TestListClipNamesEmptyTable verifies a fresh install lists nothing rather
// than erroring, so the sweep's paging loop terminates immediately.
func TestListClipNamesEmptyTable(t *testing.T) {
	t.Parallel()
	ds := setupScrubTestDB(t)

	got, err := ds.ListClipNames(t.Context(), 100, 0)
	require.NoError(t, err)
	assert.Empty(t, got)
}
