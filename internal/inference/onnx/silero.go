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

	// Reusable inference buffers and tensors, created once at construction.
	// ONNX Runtime tensors from this wrapper are backed directly by the Go
	// slices below, so Predict writes the next window into windowBuf and reads
	// the probability from probBuf without per-frame tensor allocation
	// (previously ~5 tensor create/destroy cycles per 512-sample frame). The
	// recurrent state ping-pongs between stateBufA/B: each frame reads one and
	// writes the other, so no state copy is needed either. This reuse is why
	// SileroVAD must not be used concurrently (see type comment).
	windowBuf []float32
	stateBufA []float32
	stateBufB []float32
	probBuf   []float32
	tensors   []interface{ Destroy() error }
	// ioAB runs with stateBufA as input state and stateBufB as output;
	// ioBA is the reverse. Predict alternates between them.
	inputsAB, outputsAB []ort.Value
	inputsBA, outputsBA []ort.Value
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
		return nil, fmt.Errorf("voicewatch: silero frame size must be positive, got %d", frameSize)
	}

	inputInfos, outputInfos, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("voicewatch: failed to load silero model metadata: %w", err)
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

	s := &SileroVAD{
		session:     session,
		frameSize:   frameSize,
		sampleRate:  sampleRate,
		inputNames:  inputNames,
		outputNames: outputNames,
	}
	if err := s.initTensors(); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// initTensors creates the reusable input/output tensors and the two
// alternating IO bindings. Called once from NewSileroVAD.
func (s *SileroVAD) initTensors() error {
	const stateLen = sileroStateRank * sileroBatch * sileroStateDim
	s.windowBuf = make([]float32, s.frameSize)
	s.stateBufA = make([]float32, stateLen)
	s.stateBufB = make([]float32, stateLen)
	s.probBuf = make([]float32, sileroBatch)

	inputT, err := ort.NewTensor(ort.NewShape(sileroBatch, int64(s.frameSize)), s.windowBuf)
	if err != nil {
		return fmt.Errorf("voicewatch: silero input tensor: %w", err)
	}
	s.tensors = append(s.tensors, inputT)

	stateShape := ort.NewShape(sileroStateRank, sileroBatch, sileroStateDim)
	stateTA, err := ort.NewTensor(stateShape, s.stateBufA)
	if err != nil {
		return fmt.Errorf("voicewatch: silero state tensor: %w", err)
	}
	s.tensors = append(s.tensors, stateTA)
	stateTB, err := ort.NewTensor(stateShape, s.stateBufB)
	if err != nil {
		return fmt.Errorf("voicewatch: silero stateN tensor: %w", err)
	}
	s.tensors = append(s.tensors, stateTB)

	// Silero declares "sr" as a rank-0 scalar; the ONNX Runtime wrapper cannot
	// build a zero-rank tensor (an empty shape flattens to size 0), so a [1]
	// shape is used, which the runtime accepts for the scalar input.
	srT, err := ort.NewTensor(ort.NewShape(1), []int64{s.sampleRate})
	if err != nil {
		return fmt.Errorf("voicewatch: silero sr tensor: %w", err)
	}
	s.tensors = append(s.tensors, srT)

	probT, err := ort.NewTensor(ort.NewShape(sileroBatch, 1), s.probBuf)
	if err != nil {
		return fmt.Errorf("voicewatch: silero output tensor: %w", err)
	}
	s.tensors = append(s.tensors, probT)

	bindInputs := func(stateT ort.Value) ([]ort.Value, error) {
		inputs := make([]ort.Value, len(s.inputNames))
		for i, name := range s.inputNames {
			switch name {
			case sileroInputName:
				inputs[i] = inputT
			case sileroStateName:
				inputs[i] = stateT
			case sileroSRName:
				inputs[i] = srT
			default:
				return nil, fmt.Errorf("voicewatch: unexpected silero input %q", name)
			}
		}
		return inputs, nil
	}
	bindOutputs := func(stateT ort.Value) ([]ort.Value, error) {
		outputs := make([]ort.Value, len(s.outputNames))
		for i, name := range s.outputNames {
			switch name {
			case sileroOutputName:
				outputs[i] = probT
			case sileroStateNName:
				outputs[i] = stateT
			default:
				return nil, fmt.Errorf("voicewatch: unexpected silero output %q", name)
			}
		}
		return outputs, nil
	}

	if s.inputsAB, err = bindInputs(stateTA); err != nil {
		return err
	}
	if s.outputsAB, err = bindOutputs(stateTB); err != nil {
		return err
	}
	if s.inputsBA, err = bindInputs(stateTB); err != nil {
		return err
	}
	if s.outputsBA, err = bindOutputs(stateTA); err != nil {
		return err
	}
	return nil
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

	// Fresh clip: reset the recurrent state. Only stateBufA needs zeroing —
	// the first frame reads A and fully overwrites B; every later frame fully
	// overwrites its output buffer.
	clear(s.stateBufA)
	probs := make([]float32, 0, len(clip)/frame)

	useAB := true
	for off := 0; off+frame <= len(clip); off += frame {
		copy(s.windowBuf, clip[off:off+frame])
		inputs, outputs := s.inputsBA, s.outputsBA
		if useAB {
			inputs, outputs = s.inputsAB, s.outputsAB
		}
		if err := s.session.Run(inputs, outputs); err != nil {
			return nil, fmt.Errorf("voicewatch: silero inference failed: %w", err)
		}
		probs = append(probs, s.probBuf[0])
		useAB = !useAB
	}
	return probs, nil
}

// Close releases the ONNX session and the reusable tensors.
func (s *SileroVAD) Close() error {
	var firstErr error
	for _, t := range s.tensors {
		if err := t.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.tensors = nil
	if s.session != nil {
		if err := s.session.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.session = nil
	}
	return firstErr
}
