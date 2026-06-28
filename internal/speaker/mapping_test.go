package speaker

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSoftmax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		logits []float32
		want   []float64
	}{
		{"empty", nil, []float64{}},
		{"uniform", []float32{0, 0, 0}, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}},
		{"single", []float32{5}, []float64{1}},
		// Large magnitude must not overflow thanks to max subtraction.
		{"stable-large", []float32{1000, 1000, 1000}, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := softmax(tt.logits)
			require.Len(t, got, len(tt.want))
			var sum float64
			for i := range got {
				assert.InDelta(t, tt.want[i], got[i], 1e-9)
				assert.False(t, math.IsNaN(got[i]) || math.IsInf(got[i], 0), "no NaN/Inf")
				sum += got[i]
			}
			if len(got) > 0 {
				assert.InDelta(t, 1.0, sum, 1e-9, "probabilities sum to 1")
			}
		})
	}
}

func TestSoftmaxMonotonic(t *testing.T) {
	t.Parallel()
	// Higher logit -> higher probability.
	got := softmax([]float32{1, 2, 3})
	assert.Less(t, got[0], got[1])
	assert.Less(t, got[1], got[2])
}

func TestMapGender(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		logits     []float32
		wantLabel  string
		wantConfHi bool // confidence should be the dominant softmax prob (> 0.5)
	}{
		// logits ordered [child, female, male].
		{"female-wins", []float32{0, 5, 0}, GenderFemale, true},
		{"male-wins", []float32{0, 0, 5}, GenderMale, true},
		{"child-wins-is-unknown", []float32{5, 0, 0}, GenderUnknown, true},
		// A 2-element input is handled by the 2-class path, so "too short" here
		// means fewer than 2 elements (see TestMapGender2Class for 2-class cases).
		{"too-short", []float32{1}, "", false},
		{"empty", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			label, conf := mapGender(tt.logits)
			assert.Equal(t, tt.wantLabel, label)
			if tt.wantLabel == "" {
				assert.Zero(t, conf)
				return
			}
			assert.GreaterOrEqual(t, conf, 0.0)
			assert.LessOrEqual(t, conf, 1.0)
			if tt.wantConfHi {
				assert.Greater(t, conf, 0.5, "winning class confidence should dominate")
			}
		})
	}
}

func TestMapGender2Class(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		logits     []float32
		wantLabel  string
		wantConfHi bool // confidence should be the dominant softmax prob (> 0.5)
	}{
		// logits ordered [female, male] (see gender2ClassFemaleIdx assumption).
		{"female-wins", []float32{5, 0}, GenderFemale, true},
		{"male-wins", []float32{0, 5}, GenderMale, true},
		{"tie-low-conf", []float32{1, 1}, GenderFemale, false}, // argmax -> first (female)
		{"single-element", []float32{1}, "", false},
		{"nil-logits", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			label, conf := mapGender(tt.logits)
			assert.Equal(t, tt.wantLabel, label)
			if tt.wantLabel == "" {
				assert.Zero(t, conf)
				return
			}
			assert.GreaterOrEqual(t, conf, 0.0)
			assert.LessOrEqual(t, conf, 1.0)
			if tt.wantConfHi {
				assert.Greater(t, conf, 0.5, "winning class confidence should dominate")
			} else {
				// A tie yields exactly the uniform probability (0.5 for 2 classes).
				assert.InDelta(t, 0.5, conf, 1e-9, "tie -> uniform confidence")
			}
		})
	}
}

func TestMapGender2ClassConfidenceMatchesSoftmax(t *testing.T) {
	t.Parallel()
	logits := []float32{2.0, 1.0}
	probs := softmax(logits)
	label, conf := mapGender(logits)
	// Female (index 0) is the argmax; confidence must equal its softmax prob.
	assert.Equal(t, GenderFemale, label)
	assert.InDelta(t, probs[gender2ClassFemaleIdx], conf, 1e-9)
}

func TestMapGenderConfidenceMatchesSoftmax(t *testing.T) {
	t.Parallel()
	logits := []float32{0.5, 2.0, 1.0}
	probs := softmax(logits)
	_, conf := mapGender(logits)
	// Female (index 1) is the argmax; confidence must equal its softmax prob.
	assert.InDelta(t, probs[genderClassFemale], conf, 1e-9)
}

func TestMapAgeBands(t *testing.T) {
	t.Parallel()
	// Band centers -> confidence ~1.0 (delta accommodates float32 input rounding).
	tests := []struct {
		name     string
		score    float32
		wantBand string
	}{
		{"child-center", 0.065, AgeBandChild},  // 6.5 years
		{"teen-center", 0.165, AgeBandTeen},    // 16.5 years
		{"adult-center", 0.40, AgeBandAdult},   // 40 years
		{"senior-center", 0.80, AgeBandSenior}, // 80 years
		{"young-child", 0.01, AgeBandChild},    // 1 year
		{"old-senior", 1.0, AgeBandSenior},     // 100 years
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			band, conf := mapAge(tt.score)
			assert.Equal(t, tt.wantBand, band)
			assert.GreaterOrEqual(t, conf, 0.0)
			assert.LessOrEqual(t, conf, 1.0)
		})
	}
}

func TestMapAgeBandCentersAreConfident(t *testing.T) {
	t.Parallel()
	// At a band center the prediction is maximally inside the band -> conf ~1.0.
	for _, score := range []float32{0.065, 0.165, 0.40, 0.80} {
		_, conf := mapAge(score)
		assert.InDelta(t, 1.0, conf, 1e-6)
	}
}

func TestMapAgeBoundaryIsLowConfidence(t *testing.T) {
	t.Parallel()
	// A score landing on the child/teen boundary (~13 years) yields near-zero
	// confidence regardless of which side float32 rounding places it.
	_, conf := mapAge(0.13)
	assert.Less(t, conf, 0.01, "boundary prediction should be low confidence")
}

func TestBandFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		years float64
		want  string
	}{
		{-5, AgeBandChild},
		{0, AgeBandChild},
		{12.9, AgeBandChild},
		{13, AgeBandTeen},
		{19.9, AgeBandTeen},
		{20, AgeBandAdult},
		{59.9, AgeBandAdult},
		{60, AgeBandSenior},
		{200, AgeBandSenior},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, bandFor(tt.years).band)
		})
	}
}
