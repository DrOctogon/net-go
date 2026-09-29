package entities

import "time"

// SpeakerName stores the user-assigned display name for a voice-print speaker
// cluster ("spk_<n>"). Mirrors the legacy speaker_names table so the roster
// behaves identically on both stores: one row per named speaker, upserted on
// rename, deleted when the name is cleared.
type SpeakerName struct {
	ID        uint      `gorm:"primaryKey"`
	SpeakerID string    `gorm:"uniqueIndex;not null;size:32"` // Voice-print cluster id ("spk_<n>")
	Name      string    `gorm:"not null;size:64"`             // User-assigned display name (trimmed)
	CreatedAt time.Time `gorm:"not null"`                     // When first named
	UpdatedAt time.Time `gorm:"not null"`                     // Last rename time
}
