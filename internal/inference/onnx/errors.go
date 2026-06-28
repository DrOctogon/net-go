package onnx

import (
	"errors"
	"fmt"
)

var (
	ErrModelPathRequired = errors.New("voicewatch: model path is required")
	ErrLabelsRequired    = errors.New("voicewatch: labels are required")
	ErrEmptyBatch        = errors.New("voicewatch: batch must contain at least one segment")
	ErrSessionClosed     = errors.New("voicewatch: session is closed")
)

type EmbeddingDimMismatchError struct {
	Expected int
	Got      int
}

func (e *EmbeddingDimMismatchError) Error() string {
	return fmt.Sprintf("voicewatch: embedding dimension mismatch: classifier expects %d, got %d", e.Expected, e.Got)
}

type LabelCountError struct {
	Expected int
	Got      int
}

func (e *LabelCountError) Error() string {
	return fmt.Sprintf("voicewatch: label count mismatch: model has %d classes but %d labels were provided", e.Expected, e.Got)
}

type ModelDetectionError struct {
	Reason string
}

func (e *ModelDetectionError) Error() string {
	return fmt.Sprintf("voicewatch: cannot detect model type: %s", e.Reason)
}

type LabelLoadError struct {
	Path   string
	Reason string
}

func (e *LabelLoadError) Error() string {
	return fmt.Sprintf("voicewatch: failed to load labels from %s: %s", e.Path, e.Reason)
}

// AgeGenderIOError indicates the age/gender model's inputs or outputs could not
// be unambiguously routed by tensor dimension. The audEERING export uses
// uncertain tensor names, so I/O is matched by shape; this error is returned
// when that matching is ambiguous (see agegender.go).
type AgeGenderIOError struct {
	Reason string
}

func (e *AgeGenderIOError) Error() string {
	return fmt.Sprintf("voicewatch: age/gender model I/O routing failed: %s", e.Reason)
}

