// list_clip_names_test.go: Tests for the paged clip-reference listing used by
// the disk manager's reconciliation sweep, against a real v2 SQLite database.
//
// The sweep must behave identically on both datastores, so this mirrors
// internal/datastore/list_clip_names_test.go.
package v2only

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/diskmanager"
)

// The reconciliation sweep reaches the datastore through a runtime type
// assertion to diskmanager.ReconcileStore, which fails silently (reconciliation
// is skipped) if this datastore stops satisfying it. Assert it at compile time
// so a rename here is a build failure, not a quietly disabled sweep.
var _ diskmanager.ReconcileStore = (*Datastore)(nil)

// clipNameTestNote returns a minimal detection carrying the given clip
// reference.
func clipNameTestNote(clipName string) *datastore.Note {
	return &datastore.Note{
		Date:           speechTestDate,
		Time:           speechTestTime,
		ScientificName: speechTestSciName,
		CommonName:     speechTestComName,
		Confidence:     0.9,
		ClipName:       clipName,
	}
}

// TestListClipNames verifies that only non-empty clip references are returned,
// in a stable order, and that limit/offset page through them without gaps or
// repeats.
func TestListClipNames(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := context.Background()

	// Detections with clips, interleaved with ones that have none. The
	// clipless ones must never appear: a detection with no clip is not drift.
	want := make([]string, 0, 5)
	for i := range 5 {
		clip := fmt.Sprintf("2026/07/human_%d.wav", i)
		require.NoError(t, ds.Save(clipNameTestNote(clip), nil))
		want = append(want, clip)
		require.NoError(t, ds.Save(clipNameTestNote(""), nil))
	}

	t.Run("returns only non-empty references", func(t *testing.T) {
		got, err := ds.ListClipNames(ctx, 100, 0)
		require.NoError(t, err)
		assert.Equal(t, want, got, "Detections without a clip must not be listed")
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
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()

	got, err := ds.ListClipNames(context.Background(), 100, 0)
	require.NoError(t, err)
	assert.Empty(t, got)
}
