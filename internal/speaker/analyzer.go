package speaker

import "context"

// Analyzer estimates speaker attributes from 16 kHz mono audio frames.
//
// The samples argument is channel-major ([]channel of []sample); callers pass a
// single mono channel. Implementations must be safe to call from the detection
// pipeline and must not panic on empty input. Returning an error must not abort
// the detection — callers log and continue with empty Attributes.
type Analyzer interface {
	Analyze(ctx context.Context, samples [][]float32) (Attributes, error)
}
