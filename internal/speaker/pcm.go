package speaker

import "encoding/binary"

// sampleScaleS16 maps a signed 16-bit PCM sample to the [-1, 1) range.
const sampleScaleS16 = 32768.0

// PCMS16LEToFloat32 converts little-endian signed 16-bit PCM bytes to
// normalized float32 samples in [-1, 1). A trailing odd byte (incomplete
// sample) is ignored. The detection pipeline carries the 3-second clip as
// 16 kHz mono s16le bytes; this is the conversion the analyzer input expects.
func PCMS16LEToFloat32(pcm []byte) []float32 {
	n := len(pcm) / 2
	if n == 0 {
		return nil
	}
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		out[i] = float32(s) / sampleScaleS16
	}
	return out
}
