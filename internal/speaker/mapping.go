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

	// Gender class indices within the model's [child, female, male] logits.
	genderClassChild  = 0
	genderClassFemale = 1
	genderClassMale   = 2
	genderClassCount  = 3
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

// mapGender maps the model's [child, female, male] gender logits to a speaker
// gender label and a confidence in [0,1]. The label is the softmax argmax:
// female -> GenderFemale, male -> GenderMale, child -> GenderUnknown (child is
// an age class, not a gender). Confidence is the winning class's softmax
// probability. Logits shorter than genderClassCount yield ("", 0).
func mapGender(logits []float32) (label string, confidence float64) {
	if len(logits) < genderClassCount {
		return "", 0
	}

	probs := softmax(logits[:genderClassCount])
	argmax := 0
	for i := 1; i < genderClassCount; i++ {
		if probs[i] > probs[argmax] {
			argmax = i
		}
	}
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
