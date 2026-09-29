// speaker_reassign_test.go: round-trip tests for the bulk speaker relabelling
// used by cluster merge (ReassignSpeakerID) and forget (ClearSpeakerID).
// Real SQLite, not mocks — mirrors speaker_names_test.go.
package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Speaker ids shared by the bulk relabelling tests.
const (
	reassignFromID  = "spk_2"
	reassignToID    = "spk_1"
	reassignOtherID = "spk_5"
)

func TestReassignSpeakerID_RoundTrip(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	// Two detections for the source, one for the target, one unrelated speaker.
	seedSpeakerNote(t, ds, reassignFromID)
	seedSpeakerNote(t, ds, reassignFromID)
	seedSpeakerNote(t, ds, reassignToID)
	seedSpeakerNote(t, ds, reassignOtherID)

	moved, err := ds.ReassignSpeakerID(ctx, reassignFromID, reassignToID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), moved)

	roster, err := ds.GetSpeakerRoster(ctx)
	require.NoError(t, err)
	assert.Equal(t, []SpeakerRosterEntry{
		{SpeakerID: reassignToID, Detections: 3},
		{SpeakerID: reassignOtherID, Detections: 1},
	}, roster, "source id must be gone, its detections adopted by the target")

	// Reassigning again finds nothing to move and is not an error.
	moved, err = ds.ReassignSpeakerID(ctx, reassignFromID, reassignToID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), moved)
}

func TestClearSpeakerID_RoundTrip(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	seedSpeakerNote(t, ds, reassignToID)
	seedSpeakerNote(t, ds, reassignToID)
	seedSpeakerNote(t, ds, reassignOtherID)

	cleared, err := ds.ClearSpeakerID(ctx, reassignToID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), cleared)

	roster, err := ds.GetSpeakerRoster(ctx)
	require.NoError(t, err)
	assert.Equal(t, []SpeakerRosterEntry{{SpeakerID: reassignOtherID, Detections: 1}}, roster,
		"cleared detections must drop off the roster, other speakers untouched")

	// Clearing an id with no detections is a no-op, not an error.
	cleared, err = ds.ClearSpeakerID(ctx, "spk_404")
	require.NoError(t, err)
	assert.Equal(t, int64(0), cleared)
}

func TestReassignAndClearSpeakerID_RejectEmptyID(t *testing.T) {
	t.Parallel()

	ds := createDatabase(t, createTestSettings(t))
	ctx := t.Context()

	_, err := ds.ReassignSpeakerID(ctx, "", reassignToID)
	require.ErrorIs(t, err, ErrEmptySpeakerID)
	_, err = ds.ReassignSpeakerID(ctx, reassignFromID, "")
	require.ErrorIs(t, err, ErrEmptySpeakerID)
	_, err = ds.ClearSpeakerID(ctx, "")
	require.ErrorIs(t, err, ErrEmptySpeakerID)
}
