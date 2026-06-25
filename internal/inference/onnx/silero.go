package onnx

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// Silero VAD model dimensions (v5 contract). The recurrent state is a
// [2, batch, 128] float32 tensor; inference is done on fixed-size sample windows.
const (
	sileroStateRank = 2
	sileroStateDim  = 128
	sileroBatch     = 1
)

// Silero VAD v5 tensor names. Inputs: input (audio window), state (recurrent
// LSTM state), sr (sample rate scalar). Outputs: output (speech probability),
// stateN (updated state).
const (
	sileroInputName  = "input"
	sileroStateName  = "state"
	sileroSRName     = "sr"
	sileroOutputName = "output"
	sileroStateNName = "stateN"
)

// SileroVAD wraps a Silero VAD ONNX model. The recurrent state is carried across
// the frames of a single Predict call, so SileroVAD is NOT safe for concurrent
// use; callers must serialize Predict.
type SileroVAD struct {
	session     *ort.DynamicAdvancedSession
	frameSize   int
	sampleRate  int64
	inputNames  []string
	outputNames []string
}

// NewSileroVAD loads a Silero VAD ONNX model from modelPath. frameSize is the
// per-inference window in samples (512 at 16 kHz); sampleRate is the value fed
// to the model's "sr" input. threads <= 0 uses the createSession default.
// The ONNX Runtime must be initialized before calling this.
func NewSileroVAD(modelPath string, frameSize int, sampleRate int64, threads int) (*SileroVAD, error) {
	if modelPath == "" {
		return nil, ErrModelPathRequired
	}
	if frameSize <= 0 {
		return nil, fmt.Errorf("birdnet: silero frame size must be positive, got %d", frameSize)
	}

	inputInfos, outputInfos, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("birdnet: failed to load silero model metadata: %w", err)
	}
	inputNames := make([]string, len(inputInfos))
	for i := range inputInfos {
		inputNames[i] = inputInfos[i].Name
	}
	outputNames := make([]string, len(outputInfos))
	for i := range outputInfos {
		outputNames[i] = outputInfos[i].Name
	}

	var optsFn func(*ort.SessionOptions)
	if threads > 0 {
		optsFn = func(so *ort.SessionOptions) {
			_ = so.SetIntraOpNumThreads(threads)
			_ = so.SetInterOpNumThreads(threads)
		}
	}

	session, err := createSession(modelPath, inputNames, outputNames, optsFn)
	if err != nil {
		return nil, err
	}

	return &SileroVAD{
		session:     session,
		frameSize:   frameSize,
		sampleRate:  sampleRate,
		inputNames:  inputNames,
		outputNames: outputNames,
	}, nil
}

// Predict runs the VAD over clip in frameSize-sample windows and returns one
// speech probability (0..1) per full frame. Trailing samples that do not fill a
// frame are ignored. The recurrent state is reset to zero at the start of each
// call so successive clips are analyzed independently.
func (s *SileroVAD) Predict(clip []float32) ([]float32, error) {
	if s.session == nil {
		return nil, ErrSessionClosed
	}
	frame := s.frameSize
	if len(clip) < frame {
		return nil, nil
	}

	state := make([]float32, sileroStateRank*sileroBatch*sileroStateDim) // zero-initialized
	sr := []int64{s.sampleRate}
	probs := make([]float32, 0, len(clip)/frame)

	for off := 0; off+frame <= len(clip); off += frame {
		prob, newState, err := s.runFrame(clip[off:off+frame], state, sr)
		if err != nil {
			return nil, err
		}
		probs = append(probs, prob)
		state = newState
	}
	return probs, nil
}

// runFrame runs a single VAD inference for one window, returning the speech
// probability and the updated recurrent state.
func (s *SileroVAD) runFrame(window, state []float32, sr []int64) (prob float32, newState []float32, err error) {
	inputTensor, err := ort.NewTensor(ort.NewShape(sileroBatch, int64(len(window))), window)
	if err != nil {
		return 0, nil, fmt.Errorf("birdnet: silero input tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()

	stateTensor, err := ort.NewTensor(ort.NewShape(sileroStateRank, sileroBatch, sileroStateDim), state)
	if err != nil {
		return 0, nil, fmt.Errorf("birdnet: silero state tensor: %w", err)
	}
	defer func() { _ = stateTensor.Destroy() }()

	// Silero declares "sr" as a rank-0 scalar; the ONNX Runtime wrapper cannot
	// build a zero-rank tensor (an empty shape flattens to size 0), so a [1]
	// shape is used, which the runtime accepts for the scalar input.
	srTensor, err := ort.NewTensor(ort.NewShape(1), sr)
	if err != nil {
		return 0, nil, fmt.Errorf("birdnet: silero sr tensor: %w", err)
	}
	defer func() { _ = srTensor.Destroy() }()

	outProb, err := ort.NewEmptyTensor[float32](ort.NewShape(sileroBatch, 1))
	if err != nil {
		return 0, nil, fmt.Errorf("birdnet: silero output tensor: %w", err)
	}
	defer func() { _ = outProb.Destroy() }()

	outState, err := ort.NewEmptyTensor[float32](ort.NewShape(sileroStateRank, sileroBatch, sileroStateDim))
	if err != nil {
		return 0, nil, fmt.Errorf("birdnet: silero stateN tensor: %w", err)
	}
	defer func() { _ = outState.Destroy() }()

	inputs := make([]ort.Value, len(s.inputNames))
	for i, name := range s.inputNames {
		switch name {
		case sileroInputName:
			inputs[i] = inputTensor
		case sileroStateName:
			inputs[i] = stateTensor
		case sileroSRName:
			inputs[i] = srTensor
		default:
			return 0, nil, fmt.Errorf("birdnet: unexpected silero input %q", name)
		}
	}

	outputs := make([]ort.Value, len(s.outputNames))
	for i, name := range s.outputNames {
		switch name {
		case sileroOutputName:
			outputs[i] = outProb
		case sileroStateNName:
			outputs[i] = outState
		default:
			return 0, nil, fmt.Errorf("birdnet: unexpected silero output %q", name)
		}
	}

	if err := s.session.Run(inputs, outputs); err != nil {
		return 0, nil, fmt.Errorf("birdnet: silero inference failed: %w", err)
	}

	probData := outProb.GetData()
	if len(probData) == 0 {
		return 0, nil, fmt.Errorf("birdnet: silero produced empty output")
	}
	updated := outState.GetData()
	newState = make([]float32, len(updated))
	copy(newState, updated)
	return probData[0], newState, nil
}

// Close releases the ONNX session and associated resources.
func (s *SileroVAD) Close() error {
	if s.session != nil {
		err := s.session.Destroy()
		s.session = nil
		return err
	}
	return nil
}
