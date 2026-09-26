// model_test.go: Tests for Note model schema, in particular index tags that
// AutoMigrate is expected to create.
package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNote_SpeakerIDIndexesCreated verifies AutoMigrate actually creates the
// speaker_id indexes declared via gorm tags on Note. Roster/activity/search
// queries filter and sort notes by speaker_id (see search_advanced.go), so a
// missing index here means those queries silently full-scan the notes table.
func TestNote_SpeakerIDIndexesCreated(t *testing.T) {
	ds := setupTestDB(t)

	var indexNames []string
	err := ds.DB.Raw(
		"SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='notes'",
	).Scan(&indexNames).Error
	require.NoError(t, err)

	assert.Contains(t, indexNames, "idx_notes_speakerid",
		"plain speaker_id index should be created by AutoMigrate")
	assert.Contains(t, indexNames, "idx_notes_speakerid_date",
		"composite (speaker_id, date) index should be created by AutoMigrate")
}
