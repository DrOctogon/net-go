package speaker

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCosine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"scaled-same-direction", []float32{2, 0}, []float32{8, 0}, 1},
		{"empty-a", nil, []float32{1}, 0},
		{"length-mismatch", []float32{1, 2}, []float32{1}, 0},
		{"zero-magnitude", []float32{0, 0}, []float32{1, 1}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Cosine(tt.a, tt.b)
			assert.InDelta(t, tt.want, got, 1e-6)
		})
	}
}

func TestPCMS16LEToFloat32(t *testing.T) {
	t.Parallel()

	// Build a 3-sample s16le buffer: 0, max positive, max negative.
	// Assign to int16 vars first so the negative->uint16 conversion is a runtime
	// reinterpretation, not a constant overflow.
	var zero, maxS, minS int16 = 0, math.MaxInt16, math.MinInt16
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint16(buf[0:], uint16(zero))
	binary.LittleEndian.PutUint16(buf[2:], uint16(maxS))
	binary.LittleEndian.PutUint16(buf[4:], uint16(minS))

	got := PCMS16LEToFloat32(buf)
	require.Len(t, got, 3)
	assert.InDelta(t, 0.0, got[0], 1e-6)
	assert.InDelta(t, float64(math.MaxInt16)/sampleScaleS16, got[1], 1e-6)
	assert.InDelta(t, -1.0, got[2], 1e-6)

	assert.Nil(t, PCMS16LEToFloat32(nil))
	assert.Nil(t, PCMS16LEToFloat32([]byte{0x01}), "odd trailing byte ignored -> empty")
}

func TestNoopAnalyzer(t *testing.T) {
	t.Parallel()
	attrs, err := NoopAnalyzer{}.Analyze(context.Background(), [][]float32{{0.1, 0.2}})
	require.NoError(t, err)
	assert.False(t, attrs.HasAttributes())
	assert.Empty(t, attrs.Gender)
	assert.Empty(t, attrs.AgeBand)
	assert.Nil(t, attrs.Embedding)
}

func TestNewReturnsNoopUntilModelExists(t *testing.T) {
	t.Parallel()
	a := New(Config{Enabled: true, GenderEnabled: true})
	_, ok := a.(NoopAnalyzer)
	assert.True(t, ok, "no real model vendored yet -> noop")
}

func TestValidators(t *testing.T) {
	t.Parallel()
	assert.True(t, IsValidGender(GenderMale))
	assert.True(t, IsValidGender(GenderUnknown))
	assert.False(t, IsValidGender(""))
	assert.False(t, IsValidGender("nonbinary-typo"))

	assert.True(t, IsValidAgeBand(AgeBandChild))
	assert.True(t, IsValidAgeBand(AgeBandSenior))
	assert.False(t, IsValidAgeBand(""))
	assert.False(t, IsValidAgeBand("middle"))
}

func TestHasAttributes(t *testing.T) {
	t.Parallel()
	assert.True(t, Attributes{Gender: GenderMale}.HasAttributes())
	assert.True(t, Attributes{AgeBand: AgeBandAdult}.HasAttributes())
	assert.False(t, Attributes{}.HasAttributes())
	assert.False(t, Attributes{GenderConfidence: 0.9}.HasAttributes(), "confidence without label is not an estimate")
}
