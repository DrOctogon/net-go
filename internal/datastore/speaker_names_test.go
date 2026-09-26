// speaker_names_test.go: round-trip tests for persistent speaker names.
//
// These tests use a real SQLite database (not mocks) to verify upsert,
// delete-on-empty-name, and validation behavior of the speaker roster.
package datastore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetSpeakerName_RoundTrip(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "Alice"))
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_3", "Bob"))

	names, err := ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 2)
	assert.Equal(t, "spk_1", names[0].SpeakerID)
	assert.Equal(t, "Alice", names[0].Name)
	assert.Equal(t, "spk_3", names[1].SpeakerID)
	assert.Equal(t, "Bob", names[1].Name)
}

func TestSetSpeakerName_UpsertOverwrites(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "Alice"))
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "Alicia"))

	names, err := ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 1, "upsert must not create a second row for the same speaker")
	assert.Equal(t, "Alicia", names[0].Name)
}

func TestSetSpeakerName_EmptyNameDeletes(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "Alice"))
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", ""))

	names, err := ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	assert.Empty(t, names)

	// Clearing a mapping that does not exist must be a no-op, not an error.
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_99", ""))
}

func TestSetSpeakerName_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "  Alice  "))

	names, err := ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 1)
	assert.Equal(t, "Alice", names[0].Name)

	// Whitespace-only collapses to empty -> deletes the row.
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "   "))
	names, err = ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestSetSpeakerName_Validation(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	require.Error(t, ds.SetSpeakerName(ctx, "", "Alice"), "empty speaker id must be rejected")
	require.Error(t, ds.SetSpeakerName(ctx, "spk_1", strings.Repeat("a", MaxSpeakerNameLength+1)),
		"overlong name must be rejected")

	// Exactly at the cap is allowed.
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", strings.Repeat("a", MaxSpeakerNameLength)))

	names, err := ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 1)
}

func TestGetSpeakerNames_EmptyDatabase(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))

	names, err := ds.GetSpeakerNames(t.Context())
	require.NoError(t, err)
	assert.Empty(t, names)
}

// seedSpeakerNote saves a minimal detection note carrying the given speaker id.
func seedSpeakerNote(t *testing.T, ds Interface, speakerID string) {
	t.Helper()
	note := Note{
		SourceNode:     "test-node",
		Date:           "2024-01-15",
		Time:           "14:30:45",
		ScientificName: "Human vocal",
		CommonName:     "Human vocal",
		Confidence:     0.9,
		SpeakerID:      speakerID,
	}
	require.NoError(t, ds.Save(&note, nil))
}

func TestGetSpeakerRoster_MergesCountsAndNames(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	// spk_1: named, 2 detections. spk_2: unnamed, 1 detection.
	// spk_9: named, no detections (retention-scrubbed). Plus one note with no
	// speaker id, which must not appear on the roster.
	seedSpeakerNote(t, ds, "spk_1")
	seedSpeakerNote(t, ds, "spk_1")
	seedSpeakerNote(t, ds, "spk_2")
	seedSpeakerNote(t, ds, "")
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_1", "Alice"))
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_9", "Bob"))

	roster, err := ds.GetSpeakerRoster(ctx)
	require.NoError(t, err)
	assert.Equal(t, []SpeakerRosterEntry{
		{SpeakerID: "spk_1", Name: "Alice", Detections: 2},
		{SpeakerID: "spk_2", Name: "", Detections: 1},
		{SpeakerID: "spk_9", Name: "Bob", Detections: 0},
	}, roster)
}

func TestGetSpeakerRoster_EmptyDatabase(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))

	roster, err := ds.GetSpeakerRoster(t.Context())
	require.NoError(t, err)
	assert.Empty(t, roster)
}
