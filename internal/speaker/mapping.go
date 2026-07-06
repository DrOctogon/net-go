package speaker

import "math"

// Model output mapping constants for the audEERING age/gender model.
const (
	// ageScoreToYears converts the model's age score (0..1) into years.
	ageScoreToYears = 100.0

	// Age-band upper boundaries in years. A speaker is "child" below
	// ageChildMaxYears, "teen" below ageTeenMaxYears, "adult" below
	// ageAdultMaxYears, and "senior" at or above it. ageSeniorMaxYears bounds
	// the open-ended senior band so the confidence heuristic has a finite
	// half-width (the score maxes out at 1.0 -> 100 years).
	ageChildMaxYears  = 13.0
	ageTeenMaxYears   = 20.0
	ageAdultMaxYears  = 60.0
	ageSeniorMaxYears = 100.0

	// Gender class indices within the audEERING 3-class [child, female, male]
	// logits. "child" is an age class, not a gender, so it maps to GenderUnknown.
	genderClassChild  = 0
	genderClassFemale = 1
	genderClassMale   = 2
	genderClassCount  = 3

	// 2-class gender model indices, order [male, female]. VERIFIED against the
	// JaesungHuh/voice-gender-classifier source: `self.pred2gender = {0:'male',
	// 1:'female'}`. A different 2-class export could use [female, male] — if so,
	// swap these two constants (and only these).
	gender2ClassMaleIdx   = 0
	gender2ClassFemaleIdx = 1
	gender2ClassCount     = 2
)

// ageBandRange describes one age band and its [lo, hi) years interval. The
// senior band's hi is the finite ageSeniorMaxYears cap.
type ageBandRange struct {
	band   string
	lo, hi float64
}

// ageBands lists the bands in ascending order. The last entry (senior) is the
// catch-all for any age at or above ageAdultMaxYears.
var ageBands = []ageBandRange{
	{AgeBandChild, 0, ageChildMaxYears},
	{AgeBandTeen, ageChildMaxYears, ageTeenMaxYears},
	{AgeBandAdult, ageTeenMaxYears, ageAdultMaxYears},
	{AgeBandSenior, ageAdultMaxYears, ageSeniorMaxYears},
}

// softmax returns the numerically stable softmax of logits as float64
// probabilities that sum to 1. An empty input yields an empty slice.
func softmax(logits []float32) []float64 {
	out := make([]float64, len(logits))
	if len(logits) == 0 {
		return out
	}

	maxLogit := float64(logits[0])
	for _, v := range logits[1:] {
		if float64(v) > maxLogit {
			maxLogit = float64(v)
		}
	}

	var sum float64
	for i, v := range logits {
		e := math.Exp(float64(v) - maxLogit)
		out[i] = e
		sum += e
	}
	if sum == 0 {
		return out
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// mapGender maps a gender model's output logits to a speaker gender label and a
// confidence in [0,1]. It supports both export shapes:
//
//   - 3-class [child, female, male] (audEERING order): female -> GenderFemale,
//     male -> GenderMale, child -> GenderUnknown (child is an age class, not a
//     gender). Used when len(logits) >= genderClassCount.
//   - 2-class [male, female] (verified JaesungHuh order; see
//     gender2ClassMaleIdx/gender2ClassFemaleIdx): argmax -> male/female. Used when
//     len(logits) == gender2ClassCount.
//
// Confidence is the winning class's softmax probability. Logits shorter than
// gender2ClassCount yield ("", 0).
func mapGender(logits []float32) (label string, confidence float64) {
	switch {
	case len(logits) >= genderClassCount:
		return mapGender3Class(logits)
	case len(logits) >= gender2ClassCount:
		return mapGender2Class(logits)
	default:
		return "", 0
	}
}

// mapGender3Class maps audEERING [child, female, male] logits.
func mapGender3Class(logits []float32) (label string, confidence float64) {
	probs := softmax(logits[:genderClassCount])
	argmax := argmaxFloat64(probs)
	confidence = probs[argmax]
	switch argmax {
	case genderClassFemale:
		return GenderFemale, confidence
	case genderClassMale:
		return GenderMale, confidence
	case genderClassChild:
		return GenderUnknown, confidence
	default:
		return GenderUnknown, confidence
	}
}

// mapGender2Class maps 2-class [male, female] logits (verified order; see
// gender2ClassMaleIdx/gender2ClassFemaleIdx).
func mapGender2Class(logits []float32) (label string, confidence float64) {
	probs := softmax(logits[:gender2ClassCount])
	argmax := argmaxFloat64(probs)
	confidence = probs[argmax]
	switch argmax {
	case gender2ClassFemaleIdx:
		return GenderFemale, confidence
	case gender2ClassMaleIdx:
		return GenderMale, confidence
	default:
		return GenderUnknown, confidence
	}
}

// argmaxFloat64 returns the index of the largest element. The input is assumed
// non-empty (callers pass a softmax result sized to the class count).
func argmaxFloat64(values []float64) int {
	argmax := 0
	for i := 1; i < len(values); i++ {
		if values[i] > values[argmax] {
			argmax = i
		}
	}
	return argmax
}

// mapAge maps the model's age score (0..1, where score*100 ≈ years) to an age
// band and a confidence in [0,1].
//
// AgeConfidence is a deterministic HEURISTIC, not a model-emitted probability:
// it is the distance from the predicted years to the nearest boundary of the
// chosen band, normalized by the band's half-width. A prediction at the center
// of a band scores 1.0; one right on a boundary scores 0.0 (low confidence
// because a small error would flip the band).
func mapAge(score float32) (band string, confidence float64) {
	years := float64(score) * ageScoreToYears

	b := bandFor(years)
	clamped := math.Min(math.Max(years, b.lo), b.hi)
	halfWidth := (b.hi - b.lo) / 2
	if halfWidth <= 0 {
		return b.band, 0
	}
	distToNearest := math.Min(clamped-b.lo, b.hi-clamped)
	confidence = distToNearest / halfWidth
	confidence = math.Min(math.Max(confidence, 0), 1)
	return b.band, confidence
}

// bandFor returns the age band containing years. Non-senior bands match when
// years is below their upper boundary; senior is the catch-all.
func bandFor(years float64) ageBandRange {
	for _, b := range ageBands[:len(ageBands)-1] {
		if years < b.hi {
			return b
		}
	}
	return ageBands[len(ageBands)-1]
}
