package api

import "github.com/tphakala/voicewatch/internal/speaker"

// validSpeakerGenderFilter returns g when it is a recognized gender label,
// otherwise "" (no filter). This keeps an unrecognized client value from
// silently returning zero rows and bounds the column values that reach SQL.
func validSpeakerGenderFilter(g string) string {
	if speaker.IsValidGender(g) {
		return g
	}
	return ""
}

// validSpeakerAgeBandFilter returns b when it is a recognized age-band label,
// otherwise "" (no filter).
func validSpeakerAgeBandFilter(b string) string {
	if speaker.IsValidAgeBand(b) {
		return b
	}
	return ""
}
