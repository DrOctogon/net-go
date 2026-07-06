package speaker

import "context"

// NoopAnalyzer is the default Analyzer used when the feature is disabled or no
// real model is available. It returns empty Attributes and never errors, so the
// pipeline seam is exercised without affecting detections.
type NoopAnalyzer struct{}

// Analyze implements Analyzer.
func (NoopAnalyzer) Analyze(context.Context, [][]float32) (Attributes, error) {
	return Attributes{}, nil
}
