package repository

import (
	"context"

	"github.com/tphakala/voicewatch/internal/datastore/v2/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Table name constants for the speaker roster (see tables.go for the pattern).
const (
	tableSpeakerNames   = "speaker_names"
	tableV2SpeakerNames = "v2_speaker_names"
)

// columnUpdatedAt is the shared updated-at column name used in upsert clauses.
const columnUpdatedAt = "updated_at"

// SpeakerNameRepository handles persistence of user-assigned speaker display
// names (the household speaker roster) in the v2 schema. It exists primarily
// for the legacy→v2 migration; the runtime v2only store accesses the table
// directly.
type SpeakerNameRepository interface {
	// Save upserts a speaker name by its unique speaker_id, keeping the given
	// row's Name/UpdatedAt as source of truth on conflict.
	Save(ctx context.Context, name *entities.SpeakerName) error
	// GetAll returns all speaker names ordered by speaker id.
	GetAll(ctx context.Context) ([]entities.SpeakerName, error)
}

// speakerNameRepository is the GORM implementation of SpeakerNameRepository.
type speakerNameRepository struct {
	db          *gorm.DB
	useV2Prefix bool
}

// NewSpeakerNameRepository creates a new speaker name repository.
func NewSpeakerNameRepository(db *gorm.DB, useV2Prefix bool) SpeakerNameRepository {
	return &speakerNameRepository{db: db, useV2Prefix: useV2Prefix}
}

func (r *speakerNameRepository) tableName() string {
	if r.useV2Prefix {
		return tableV2SpeakerNames
	}
	return tableSpeakerNames
}

// Save upserts a speaker name row keyed by speaker_id.
func (r *speakerNameRepository) Save(ctx context.Context, name *entities.SpeakerName) error {
	return r.db.WithContext(ctx).Table(r.tableName()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "speaker_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", columnUpdatedAt}),
	}).Create(name).Error
}

// GetAll returns every speaker name ordered by speaker id.
func (r *speakerNameRepository) GetAll(ctx context.Context) ([]entities.SpeakerName, error) {
	var rows []entities.SpeakerName
	err := r.db.WithContext(ctx).Table(r.tableName()).Order("speaker_id").Find(&rows).Error
	return rows, err
}
