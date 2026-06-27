// Package speaker provides speaker-attribute estimation (gender, relative age
// band) and voice-print embeddings for human-voice detections.
//
// These are demographic *estimates* derived from acoustic features, not
// biometric identity recognition. Accuracy varies with accent, language, and
// recording quality. The feature is opt-in and disabled by default; callers
// must surface the estimate/bias caveats in any user-facing context.
package speaker

// Gender estimate labels. Unknown is used when a model runs but cannot decide.
const (
	GenderMale    = "male"
	GenderFemale  = "female"
	GenderUnknown = "unknown"
)

// AgeBand estimate labels. These express *relative* age, not exact years.
const (
	AgeBandChild  = "child"
	AgeBandTeen   = "teen"
	AgeBandAdult  = "adult"
	AgeBandSenior = "senior"
)

// Attributes holds the estimated speaker attributes for a single detection.
// All fields are zero-valued when the corresponding analysis is disabled or the
// model could not produce an estimate.
type Attributes struct {
	Gender           string    // one of Gender*; "" when not estimated
	GenderConfidence float64   // 0..1
	AgeBand          string    // one of AgeBand*; "" when not estimated
	AgeConfidence    float64   // 0..1
	Embedding        []float32 // voice-print vector; nil when not computed
	SpeakerID        string    // cluster/speaker id; "" until clustering assigns one
}

// HasAttributes reports whether any gender or age estimate is populated.
func (a Attributes) HasAttributes() bool {
	return a.Gender != "" || a.AgeBand != ""
}

// IsValidGender reports whether g is a recognized gender label. The empty
// string is not valid (callers treat "" as "no filter / not estimated").
func IsValidGender(g string) bool {
	switch g {
	case GenderMale, GenderFemale, GenderUnknown:
		return true
	default:
		return false
	}
}

// IsValidAgeBand reports whether b is a recognized age-band label.
func IsValidAgeBand(b string) bool {
	switch b {
	case AgeBandChild, AgeBandTeen, AgeBandAdult, AgeBandSenior:
		return true
	default:
		return false
	}
}
