// aggregate.go contains the pure, model-free aggregation logic for the human
// voice detector. It is factored out of the ONNX inference path so it can be
// unit-tested without a real Silero VAD model binary present.
package humanvoice

import "github.com/tphakala/voicewatch/internal/datastore"

// Confidence bounds for an aggregated speech probability.
const (
	minConfidence float32 = 0.0
	maxConfidence float32 = 1.0
)

// aggregateSpeechProbability collapses per-window/per-frame Silero VAD speech
// probabilities into a single clip-level confidence using the maximum over all
// frames. A clip is considered to contain human voice if any frame does, so the
// peak frame probability is the most sensitive aggregate. The result is clamped
// to [0, 1]. An empty input yields minConfidence (0.0).
func aggregateSpeechProbability(frameProbs []float32) float32 {
	best := minConfidence
	for _, p := range frameProbs {
		if p > best {
			best = p
		}
	}
	return clampConfidence(best)
}

// clampConfidence bounds a probability to the closed interval [0, 1].
func clampConfidence(v float32) float32 {
	switch {
	case v < minConfidence:
		return minConfidence
	case v > maxConfidence:
		return maxConfidence
	default:
		return v
	}
}

// aggregateClip builds the single per-clip detection result for the human voice
// model from raw per-frame speech probabilities. It always returns exactly one
// result labeled labelHumanVoice; the processor applies the configured
// threshold downstream, so no filtering happens here.
func aggregateClip(frameProbs []float32) datastore.Results {
	return datastore.Results{
		Species:    labelHumanVoice,
		Confidence: aggregateSpeechProbability(frameProbs),
		RawLabel:   labelHumanVoice,
	}
}
