package onnx

import (
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"
)

// Per-attribute speaker model ONNX contract.
//
// Each speaker attribute (gender, age, voice-print) is served by its own
// independent ONNX model loaded at runtime from a user-supplied path (the
// weights are not vendored or embedded). A model is expected to have exactly
// ONE float32 audio input and exactly ONE float32 output vector. It expects the
// ONNX Runtime global environment to already be initialized (the Silero VAD path
// does this at startup; see runtime.go).
//
// INPUT: a single float32 audio tensor of shape (1, N) holding a 16 kHz mono raw
// waveform.
//
// PREPROCESSING: input normalization is OPT-IN per model (the `normalize` arg).
//   - normalize=false (default): the raw waveform is passed through unchanged.
//     Correct for models that do their own front end inside the ONNX graph — e.g.
//     JaesungHuh/voice-gender-classifier (ECAPA-TDNN), whose source applies
//     preemphasis + mel-filterbank + mean normalization internally and expects a
//     raw waveform in.
//   - normalize=true: per-clip zero-mean / unit-variance normalization is applied
//     (see normalizeWaveform). Correct for a Wav2Vec2-style front end (audEERING)
//     that expects an externally normalized waveform.
// Applying the wrong choice degrades accuracy, so the caller must match it to the
// model's reference pipeline.
//
// OUTPUT: a single float32 vector. Its length is interpreted by the caller:
//   - gender model:      class logits/probabilities (2 or 3 classes)
//   - age model:         a single age score in element 0
//   - voice-print model: the embedding vector (any dimension)
//
// ASSUMPTION (unverifiable in this sandbox): real exports such as
// JaesungHuh/voice-gender-classifier (ECAPA-TDNN, 16 kHz, 2-class) and audEERING
// wav2vec2 expose exactly one float32 input and one float32 output. Models that
// emit auxiliary outputs (e.g. a separate embedding head) will fail the
// single-float-output check in NewSingleOutputAudioModel and must be re-exported
// to a single output, or wrapped by a dedicated model type.
const (
	// singleOutputBatch is the batch dimension; models run one clip at a time.
	singleOutputBatch = 1
	// waveformNormEps is the per-clip unit-variance epsilon (variance + eps).
	waveformNormEps = 1e-7
)

// SingleOutputAudioModel wraps an ONNX model that takes one float32 waveform
// input and produces one float32 output vector. It is used for each independent
// per-attribute speaker model (gender, age, voice-print).
//
// NOT goroutine-safe: the session is shared and inference is stateful at the
// runtime level, so callers MUST serialize Infer.
type SingleOutputAudioModel struct {
	session    *ort.DynamicAdvancedSession
	inputName  string // the single float32 audio input
	outputName string // the single float32 output
	normalize  bool   // apply waveform zero-mean/unit-variance before inference
}

// NewSingleOutputAudioModel loads a single-input/single-output audio model from
// modelPath. It reads the model metadata, requires exactly one float32 input and
// exactly one float32 output, and creates the inference session. threads <= 0
// uses the createSession default. normalize selects waveform preprocessing (see
// the PREPROCESSING note above): false passes the raw waveform through (for
// models with an in-graph front end such as ECAPA), true applies Wav2Vec2-style
// zero-mean/unit-variance normalization. The ONNX Runtime must be initialized
// before calling.
func NewSingleOutputAudioModel(modelPath string, threads int, normalize bool) (*SingleOutputAudioModel, error) {
	if modelPath == "" {
		return nil, ErrModelPathRequired
	}

	inputInfos, outputInfos, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("voicewatch: failed to load speaker model metadata: %w", err)
	}

	inputName, err := routeAudioInput(inputInfos)
	if err != nil {
		return nil, err
	}

	outputName, err := routeSingleFloatOutput(outputInfos)
	if err != nil {
		return nil, err
	}

	var optsFn func(*ort.SessionOptions)
	if threads > 0 {
		optsFn = func(so *ort.SessionOptions) {
			_ = so.SetIntraOpNumThreads(threads)
			_ = so.SetInterOpNumThreads(threads)
		}
	}

	session, err := createSession(modelPath, []string{inputName}, []string{outputName}, optsFn)
	if err != nil {
		return nil, err
	}

	return &SingleOutputAudioModel{
		session:    session,
		inputName:  inputName,
		outputName: outputName,
		normalize:  normalize,
	}, nil
}

// routeAudioInput selects the single float32 input as the audio waveform input.
// It returns a SpeakerModelIOError when there is not exactly one float32 input.
func routeAudioInput(inputs []ort.InputOutputInfo) (string, error) {
	name := ""
	count := 0
	for i := range inputs {
		if inputs[i].DataType == ort.TensorElementDataTypeFloat {
			name = inputs[i].Name
			count++
		}
	}
	if count != 1 {
		return "", &SpeakerModelIOError{
			Reason: fmt.Sprintf("expected exactly one float32 input, found %d", count),
		}
	}
	return name, nil
}

// routeSingleFloatOutput selects the single float32 output vector. It returns a
// SpeakerModelIOError when there is not exactly one float32 output.
func routeSingleFloatOutput(outputs []ort.InputOutputInfo) (string, error) {
	name := ""
	count := 0
	for i := range outputs {
		if outputs[i].DataType == ort.TensorElementDataTypeFloat {
			name = outputs[i].Name
			count++
		}
	}
	if count != 1 {
		return "", &SpeakerModelIOError{
			Reason: fmt.Sprintf("expected exactly one float32 output, found %d", count),
		}
	}
	return name, nil
}

// Infer runs one inference over a 16 kHz mono waveform and returns a freshly
// allocated copy of the model's output vector (owned by the caller). NOT
// goroutine-safe.
//
// DYNAMIC OUTPUT HANDLING: the output length is unknown at load time (it varies
// by attribute model, and some exports size the output from the input length).
// Rather than pre-allocating an output tensor from a declared dimension (which
// fails for dynamic dims), Infer passes a nil output to
// DynamicAdvancedSession.Run, which auto-allocates a correctly-sized output
// tensor and writes it back into the slice (yalue/onnxruntime_go v1.30.1
// behavior). The auto-allocated Value is a *ort.Tensor[float32] (float outputs);
// its data is copied out and the Value is destroyed before returning.
func (m *SingleOutputAudioModel) Infer(samples []float32) ([]float32, error) {
	if m.session == nil {
		return nil, ErrSessionClosed
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("voicewatch: speaker inference requires non-empty samples")
	}

	input := samples
	if m.normalize {
		input = normalizeWaveform(samples)
	}

	inputTensor, err := ort.NewTensor(ort.NewShape(singleOutputBatch, int64(len(input))), input)
	if err != nil {
		return nil, fmt.Errorf("voicewatch: speaker input tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()

	// nil output => the runtime allocates the output tensor at the correct size.
	outputs := []ort.Value{nil}
	if err := m.session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, fmt.Errorf("voicewatch: speaker inference failed: %w", err)
	}

	out := outputs[0]
	if out == nil {
		return nil, fmt.Errorf("voicewatch: speaker inference produced no output")
	}
	defer func() { _ = out.Destroy() }()

	tensor, ok := out.(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("voicewatch: speaker output is not a float32 tensor (got %T)", out)
	}

	data := tensor.GetData()
	if len(data) == 0 {
		return nil, fmt.Errorf("voicewatch: speaker inference produced empty output")
	}

	result := make([]float32, len(data))
	copy(result, data)
	return result, nil
}

// normalizeWaveform applies per-clip zero-mean, unit-variance normalization:
// (x - mean) / sqrt(variance + eps). It returns a new slice and does not mutate
// the input.
func normalizeWaveform(samples []float32) []float32 {
	out := make([]float32, len(samples))
	if len(samples) == 0 {
		return out
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s)
	}
	mean := sum / float64(len(samples))

	var varSum float64
	for _, s := range samples {
		d := float64(s) - mean
		varSum += d * d
	}
	variance := varSum / float64(len(samples))
	denom := math.Sqrt(variance + waveformNormEps)

	for i, s := range samples {
		out[i] = float32((float64(s) - mean) / denom)
	}
	return out
}

// Close releases the ONNX session and associated resources.
func (m *SingleOutputAudioModel) Close() error {
	if m.session != nil {
		err := m.session.Destroy()
		m.session = nil
		return err
	}
	return nil
}
