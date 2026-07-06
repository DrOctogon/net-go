package onnx

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// createSession builds an ONNX Runtime session with default options.
// The caller may supply a sessionOptsFn to override thread counts or add
// execution providers; nil means use the defaults (1 intra-op thread,
// 1 inter-op thread).
func createSession(modelPath string, inputNames, outputNames []string, sessionOptsFn func(*ort.SessionOptions)) (*ort.DynamicAdvancedSession, error) {
	sessOpts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("voicewatch: failed to create session options: %w", err)
	}
	defer func() { _ = sessOpts.Destroy() }()

	if err := sessOpts.SetIntraOpNumThreads(1); err != nil {
		return nil, fmt.Errorf("voicewatch: failed to set intra-op threads: %w", err)
	}
	if err := sessOpts.SetInterOpNumThreads(1); err != nil {
		return nil, fmt.Errorf("voicewatch: failed to set inter-op threads: %w", err)
	}

	if sessionOptsFn != nil {
		sessionOptsFn(sessOpts)
	}

	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, sessOpts)
	if err != nil {
		return nil, fmt.Errorf("voicewatch: failed to create ONNX session: %w", err)
	}
	return session, nil
}
