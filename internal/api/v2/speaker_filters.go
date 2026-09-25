package api

import (
	"regexp"

	"github.com/tphakala/voicewatch/internal/speaker"
)

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

// speakerIDPattern matches voice-print cluster ids as issued by
// speaker.Clusterer ("spk_" + decimal counter).
var speakerIDPattern = regexp.MustCompile(`^spk_\d+$`)

// validSpeakerIDFilter returns id when it matches the cluster-id format,
// otherwise "" (no filter).
func validSpeakerIDFilter(id string) string {
	if speakerIDPattern.MatchString(id) {
		return id
	}
	return ""
}
