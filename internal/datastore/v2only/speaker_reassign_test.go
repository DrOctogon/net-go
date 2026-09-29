// speaker_reassign_test.go: round-trip tests for the v2 bulk speaker
// relabelling used by cluster merge (ReassignSpeakerID) and forget
// (ClearSpeakerID), mirroring the legacy speaker_reassign_test.go scenarios so
// both stores keep identical semantics.
package v2only

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/datastore"
)

// reassignOtherID is a bystander speaker that must never be touched.
const reassignOtherID = "spk_5"

// seedSpeakerNotes saves one detection per speaker id in the given order.
func seedSpeakerNotes(t *testing.T, ds *Datastore, speakerIDs ...string) {
	t.Helper()
	for i, spk := range speakerIDs {
		n := speakerTestNote("reassign_"+strconv.Itoa(i)+".wav", spk, speechTestDate, speechTestTime)
		require.NoError(t, ds.Save(n, nil))
	}
}

func TestReassignSpeakerID_RoundTrip(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	// Two detections for the source (spk_2), one for the target (spk_1),
	// one bystander.
	seedSpeakerNotes(t, ds, testSpk2, testSpk2, testSpk1, reassignOtherID)

	moved, err := ds.ReassignSpeakerID(ctx, testSpk2, testSpk1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), moved)

	roster, err := ds.GetSpeakerRoster(ctx)
	require.NoError(t, err)
	assert.Equal(t, []datastore.SpeakerRosterEntry{
		{SpeakerID: testSpk1, Detections: 3},
		{SpeakerID: reassignOtherID, Detections: 1},
	}, roster, "source id must be gone, its detections adopted by the target")

	// Reassigning again finds nothing to move and is not an error.
	moved, err = ds.ReassignSpeakerID(ctx, testSpk2, testSpk1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), moved)
}

func TestClearSpeakerID_RoundTrip(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	seedSpeakerNotes(t, ds, testSpk1, testSpk1, reassignOtherID)

	cleared, err := ds.ClearSpeakerID(ctx, testSpk1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), cleared)

	roster, err := ds.GetSpeakerRoster(ctx)
	require.NoError(t, err)
	assert.Equal(t, []datastore.SpeakerRosterEntry{{SpeakerID: reassignOtherID, Detections: 1}}, roster,
		"cleared detections must drop off the roster, other speakers untouched")

	// Clearing an id with no detections is a no-op, not an error.
	cleared, err = ds.ClearSpeakerID(ctx, "spk_404")
	require.NoError(t, err)
	assert.Equal(t, int64(0), cleared)
}

func TestReassignAndClearSpeakerID_RejectEmptyID(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	_, err := ds.ReassignSpeakerID(ctx, "", testSpk1)
	require.ErrorIs(t, err, datastore.ErrEmptySpeakerID)
	_, err = ds.ReassignSpeakerID(ctx, testSpk2, "")
	require.ErrorIs(t, err, datastore.ErrEmptySpeakerID)
	_, err = ds.ClearSpeakerID(ctx, "")
	require.ErrorIs(t, err, datastore.ErrEmptySpeakerID)
}
