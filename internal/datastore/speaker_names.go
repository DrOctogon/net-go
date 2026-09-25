// speaker_names.go: Database operations for the household speaker roster.
// Persists user-assigned display names for voice-print speaker clusters
// (ids like "spk_3") so names survive application restarts.
package datastore

import (
	"context"
	"strings"
	"time"

	"github.com/tphakala/voicewatch/internal/errors"
	"gorm.io/gorm/clause"
)

// speakerNamesUpdatedAtColumn is the updated_at column name used in the
// upsert's conflict assignment list.
const speakerNamesUpdatedAtColumn = "updated_at"

// GetSpeakerNames returns all user-assigned speaker names ordered by speaker id.
// An empty roster returns an empty slice, not an error.
func (ds *DataStore) GetSpeakerNames(ctx context.Context) ([]SpeakerName, error) {
	var names []SpeakerName
	err := ds.DB.WithContext(ctx).Order("speaker_id").Find(&names).Error
	if err != nil {
		return nil, dbError(err, "get_speaker_names", errors.PriorityMedium,
			"table", "speaker_names",
			"action", "list_speaker_roster")
	}
	return names, nil
}

// SetSpeakerName upserts the display name for a speaker cluster id.
// The name is trimmed; an empty (or whitespace-only) name deletes the mapping.
// Clearing a mapping that does not exist is a no-op.
func (ds *DataStore) SetSpeakerName(ctx context.Context, speakerID, name string) error {
	if speakerID == "" {
		return validationError("speaker id cannot be empty", "speaker_id", "")
	}

	name = strings.TrimSpace(name)
	if len(name) > MaxSpeakerNameLength {
		return validationError("name exceeds maximum length", "name", len(name))
	}

	if name == "" {
		// Empty name clears the mapping.
		return RetryOnLock(ctx, "delete_speaker_name", func() error {
			result := ds.DB.WithContext(ctx).Where("speaker_id = ?", speakerID).Delete(&SpeakerName{})
			if result.Error != nil {
				return dbError(result.Error, "delete_speaker_name", errors.PriorityMedium,
					"speaker_id", speakerID,
					"table", "speaker_names",
					"action", "clear_speaker_name")
			}
			return nil
		}, ds.getMetrics())
	}

	now := time.Now()
	entry := SpeakerName{
		SpeakerID: speakerID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Upsert on the unique speaker_id index: rename in place, keep CreatedAt.
	return RetryOnLock(ctx, "set_speaker_name", func() error {
		result := ds.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "speaker_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", speakerNamesUpdatedAtColumn}),
		}).Create(&entry)
		if result.Error != nil {
			return dbError(result.Error, "set_speaker_name", errors.PriorityMedium,
				"speaker_id", speakerID,
				"table", "speaker_names",
				"action", "persist_speaker_name")
		}
		return nil
	}, ds.getMetrics())
}
