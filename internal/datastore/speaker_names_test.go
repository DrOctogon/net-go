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
