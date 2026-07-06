package onnx

import (
	"math"
	"slices"
)

func sigmoid(x float32) float32 {
	return 1.0 / (1.0 + float32(math.Exp(float64(-x))))
}

func sigmoidSlice(logits []float32) []float32 {
	result := make([]float32, len(logits))
	for i, v := range logits {
		result[i] = sigmoid(v)
	}
	return result
}

func topK(scores []float32, labels []string, k int, minConf float32) []Prediction {
	var preds []Prediction
	n := min(len(scores), len(labels))
	for i := range n {
		if scores[i] >= minConf {
			preds = append(preds, Prediction{
				Species:    labels[i],
				Confidence: scores[i],
				Index:      i,
			})
		}
	}
	slices.SortFunc(preds, func(a, b Prediction) int {
		if a.Confidence > b.Confidence {
			return -1
		}
		if a.Confidence < b.Confidence {
			return 1
		}
		return 0
	})
	if k > 0 && len(preds) > k {
		preds = preds[:k]
	}
	return preds
}
