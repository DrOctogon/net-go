// speaker_names_test.go: round-trip tests for the v2 speaker roster methods
// (phase 2 of the speech port), mirroring the legacy speaker_names_test.go
// scenarios so both stores keep identical semantics.
package v2only

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/datastore"
)

const (
	testSpk1 = "spk_1"
	testSpk2 = "spk_2"
	testDay1 = "2026-07-01"
	testDay2 = "2026-07-02"
)

func speakerTestNote(clipName, speakerID, date, timeOfDay string) *datastore.Note {
	n := speechTestNote(clipName)
	n.SpeakerID = speakerID
	n.Date = date
	n.Time = timeOfDay
	return n
}

func TestSetSpeakerName_RoundTrip(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	require.NoError(t, ds.SetSpeakerName(ctx, testSpk1, "Alice"))
	require.NoError(t, ds.SetSpeakerName(ctx, testSpk2, "Bob"))

	names, err := ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 2)
	assert.Equal(t, testSpk1, names[0].SpeakerID)
	assert.Equal(t, "Alice", names[0].Name)
	assert.Equal(t, "Bob", names[1].Name)

	// Rename upserts in place — no duplicate row.
	require.NoError(t, ds.SetSpeakerName(ctx, testSpk1, "Alicia"))
	names, err = ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 2)
	assert.Equal(t, "Alicia", names[0].Name)

	// Empty name clears; clearing a missing mapping is a no-op.
	require.NoError(t, ds.SetSpeakerName(ctx, testSpk1, "  "))
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_99", ""))
	names, err = ds.GetSpeakerNames(ctx)
	require.NoError(t, err)
	require.Len(t, names, 1)
	assert.Equal(t, testSpk2, names[0].SpeakerID)
}

func TestSetSpeakerName_ValidationSharedWithLegacy(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	require.Error(t, ds.SetSpeakerName(ctx, "", "Alice"), "empty speaker id rejected")
	require.Error(t, ds.SetSpeakerName(ctx, testSpk1, "bad\x1b[31mansi"), "control chars rejected")
	require.Error(t, ds.SetSpeakerName(ctx, testSpk1, string([]byte{0xff, 0xfe})), "invalid UTF-8 rejected")
	require.NoError(t, ds.SetSpeakerName(ctx, testSpk1, "Grandma 👵"), "emoji allowed")
}

func TestGetSpeakerRoster_MergesCountsAndNames(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	// Two detections for spk_1, one for spk_2, one with no speaker.
	for i, spk := range []string{testSpk1, testSpk1, testSpk2, ""} {
		n := speakerTestNote("clip_"+strconv.Itoa(i)+".wav", spk, speechTestDate, speechTestTime)
		require.NoError(t, ds.Save(n, nil))
	}
	require.NoError(t, ds.SetSpeakerName(ctx, testSpk1, "Alice"))
	// Named speaker with zero detections (retention-scrubbed) stays listed.
	require.NoError(t, ds.SetSpeakerName(ctx, "spk_9", "Ghost"))

	roster, err := ds.GetSpeakerRoster(ctx)
	require.NoError(t, err)
	require.Len(t, roster, 3)

	assert.Equal(t, datastore.SpeakerRosterEntry{SpeakerID: testSpk1, Name: "Alice", Detections: 2}, roster[0])
	assert.Equal(t, datastore.SpeakerRosterEntry{SpeakerID: testSpk2, Name: "", Detections: 1}, roster[1])
	assert.Equal(t, datastore.SpeakerRosterEntry{SpeakerID: "spk_9", Name: "Ghost", Detections: 0}, roster[2])
}

func TestGetSpeakerDailyActivity_GroupsAndFilters(t *testing.T) {
	ds, cleanup := setupTestDatastore(t)
	defer cleanup()
	ctx := t.Context()

	fixtures := []struct{ spk, date string }{
		{testSpk1, testDay1}, {testSpk1, testDay1},
		{testSpk1, testDay2},
		{testSpk2, testDay2},
		{"", testDay2}, // no speaker: excluded
	}
	for i, f := range fixtures {
		n := speakerTestNote("act_"+strconv.Itoa(i)+".wav", f.spk, f.date, "12:00:00")
		require.NoError(t, ds.Save(n, nil))
	}

	all, err := ds.GetSpeakerDailyActivity(ctx, "", "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, datastore.SpeakerDailyActivity{SpeakerID: testSpk1, Date: testDay1, Count: 2}, all[0])
	assert.Equal(t, datastore.SpeakerDailyActivity{SpeakerID: testSpk1, Date: testDay2, Count: 1}, all[1])
	assert.Equal(t, datastore.SpeakerDailyActivity{SpeakerID: testSpk2, Date: testDay2, Count: 1}, all[2])

	// Inclusive range filter narrows to the second day only.
	day2, err := ds.GetSpeakerDailyActivity(ctx, testDay2, testDay2)
	require.NoError(t, err)
	require.Len(t, day2, 2)
	for _, row := range day2 {
		assert.Equal(t, testDay2, row.Date)
	}
}
