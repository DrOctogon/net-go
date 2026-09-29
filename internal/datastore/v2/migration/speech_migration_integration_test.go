//go:build integration

// speech_migration_integration_test.go: Integration tests for phase 2b of the
// v2 speech port — verifies the legacy→v2 migration carries the 10 speech
// columns on detections and migrates legacy speaker_names rows into the v2
// speaker_names table (auxiliary-migration pattern).
package migration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/datastore/v2/migration/testutil"
)

func TestMigration_SpeechFieldsCarried(t *testing.T) {
	t.Parallel()

	ctx := testutil.SetupIntegrationTest(t)

	// Seed via GORM (not the raw-SQL seeder) so the speech columns are written.
	speechNote := &datastore.Note{
		Date: "2026-06-25", Time: "10:00:00",
		ScientificName: "Homo sapiens", CommonName: "Human Voice",
		Confidence:          0.9,
		ClipName:            "2026/06/human_1.wav",
		Transcript:          "the house is on fire",
		TranscriptLang:      "en",
		Flagged:             true,
		KeywordsHit:         "fire",
		Gender:              "male",
		GenderConfidence:    0.8,
		AgeBand:             "adult",
		AgeConfidence:       0.7,
		SpeakerID:           "spk_3",
		VoicePrintEmbedding: []float32{0.1, 0.2, 0.3},
	}
	plainNote := &datastore.Note{
		Date: "2026-06-25", Time: "11:00:00",
		ScientificName: "Homo sapiens", CommonName: "Human Voice",
		Confidence: 0.5,
	}
	require.NoError(t, ctx.LegacyGorm.Create(speechNote).Error)
	require.NoError(t, ctx.LegacyGorm.Create(plainNote).Error)

	// Run migration
	ctx.StartMigration(t, 2)
	ctx.WaitForCompletion(t, 30*time.Second)

	c := context.Background()
	assert.Equal(t, int64(2), ctx.GetV2DetectionCount(t))

	// Speech-carrying detection: all 10 columns must survive.
	det, err := ctx.DetectionRepo.Get(c, speechNote.ID)
	require.NoError(t, err)
	require.NotNil(t, det)
	require.NotNil(t, det.Transcript)
	assert.Equal(t, "the house is on fire", *det.Transcript)
	require.NotNil(t, det.TranscriptLang)
	assert.Equal(t, "en", *det.TranscriptLang)
	assert.True(t, det.Flagged)
	require.NotNil(t, det.KeywordsHit)
	assert.Equal(t, "fire", *det.KeywordsHit)
	require.NotNil(t, det.Gender)
	assert.Equal(t, "male", *det.Gender)
	require.NotNil(t, det.GenderConfidence)
	assert.InDelta(t, 0.8, *det.GenderConfidence, 0.001)
	require.NotNil(t, det.AgeBand)
	assert.Equal(t, "adult", *det.AgeBand)
	require.NotNil(t, det.AgeConfidence)
	assert.InDelta(t, 0.7, *det.AgeConfidence, 0.001)
	require.NotNil(t, det.SpeakerID)
	assert.Equal(t, "spk_3", *det.SpeakerID)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, det.VoicePrintEmbedding)

	// VAD-only detection: speech columns stay NULL/zero.
	plain, err := ctx.DetectionRepo.Get(c, plainNote.ID)
	require.NoError(t, err)
	require.NotNil(t, plain)
	assert.Nil(t, plain.Transcript)
	assert.Nil(t, plain.TranscriptLang)
	assert.False(t, plain.Flagged)
	assert.Nil(t, plain.KeywordsHit)
	assert.Nil(t, plain.Gender)
	assert.Nil(t, plain.GenderConfidence)
	assert.Nil(t, plain.AgeBand)
	assert.Nil(t, plain.AgeConfidence)
	assert.Nil(t, plain.SpeakerID)
	assert.Empty(t, plain.VoicePrintEmbedding)
}

func TestMigration_SpeakerNames(t *testing.T) {
	t.Parallel()

	ctx := testutil.SetupIntegrationTest(t)

	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	legacyNames := []datastore.SpeakerName{
		{SpeakerID: "spk_1", Name: "Alice", CreatedAt: created, UpdatedAt: updated},
		{SpeakerID: "spk_2", Name: "Bob", CreatedAt: created, UpdatedAt: updated},
	}
	for i := range legacyNames {
		require.NoError(t, ctx.LegacyGorm.Create(&legacyNames[i]).Error)
	}

	// Run the auxiliary migration directly (speaker names are auxiliary data).
	c := context.Background()
	result, err := ctx.AuxiliaryMigrator.MigrateAll(c)
	require.NoError(t, err)
	assert.Equal(t, 2, result.SpeakerNames.Total)
	assert.Equal(t, 2, result.SpeakerNames.Migrated)
	assert.Equal(t, 0, result.SpeakerNames.Skipped)
	require.NoError(t, result.SpeakerNames.Error)

	v2Names, err := ctx.SpeakerNameRepo.GetAll(c)
	require.NoError(t, err)
	require.Len(t, v2Names, 2)
	assert.Equal(t, "spk_1", v2Names[0].SpeakerID)
	assert.Equal(t, "Alice", v2Names[0].Name)
	assert.Equal(t, "spk_2", v2Names[1].SpeakerID)
	assert.Equal(t, "Bob", v2Names[1].Name)

	// Re-running the migration must be idempotent (upsert by speaker_id),
	// including after a legacy rename.
	require.NoError(t, ctx.LegacyGorm.Model(&datastore.SpeakerName{}).
		Where("speaker_id = ?", "spk_2").Update("name", "Bobby").Error)

	result, err = ctx.AuxiliaryMigrator.MigrateAll(c)
	require.NoError(t, err)
	assert.Equal(t, 2, result.SpeakerNames.Migrated)

	v2Names, err = ctx.SpeakerNameRepo.GetAll(c)
	require.NoError(t, err)
	require.Len(t, v2Names, 2)
	assert.Equal(t, "Bobby", v2Names[1].Name)
}
