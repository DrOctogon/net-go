package onnx

import (
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"
)

// audEERING wav2vec2-large-robust-24-ft-age-gender ONNX export contract.
//
// The model is loaded at runtime from a user-supplied path (the weights are CC
// BY-NC-SA / non-commercial, so they are never vendored or embedded). It expects
// the ONNX Runtime global environment to already be initialized (the Silero VAD
// path does this at startup; see runtime.go).
//
// INPUT: a single float32 audio tensor of shape (1, N) holding a 16 kHz mono raw
// waveform. Standard Wav2Vec2 per-clip zero-mean / unit-variance normalization
// is applied before inference (see normalizeWaveform).
//
// OUTPUTS (3): the exact tensor names vary across the Zenodo exports, so outputs
// are routed by their trailing (feature) dimension rather than by name:
//   - age:       feature dim 1   -> score in 0..1 (×100 ≈ years)
//   - gender:    feature dim 3   -> logits [child, female, male]
//   - embedding: largest dim     -> hidden-state voice-print vector (e.g. 768)
const (
	ageGenderBatch     = 1    // models run one clip at a time
	ageGenderAgeDim    = 1    // age output trailing dimension
	ageGenderGenderDim = 3    // gender output trailing dimension [child, female, male]
	ageGenderNormEps   = 1e-7 // Wav2Vec2 unit-variance epsilon (variance + eps)
)

// AgeGenderModel wraps the audEERING age/gender/voice-print ONNX model. A single
// model file produces all three outputs.
//
// NOT goroutine-safe: the session is shared and inference is stateful at the
// runtime level, so callers MUST serialize Infer.
type AgeGenderModel struct {
	session     *ort.DynamicAdvancedSession
	inputName   string   // the single float32 audio input
	outputNames []string // session output order
	ageIdx      int      // index of the age output within outputNames
	genderIdx   int      // index of the gender output within outputNames
	embedIdx    int      // index of the embedding output within outputNames
	embedDim    int64    // embedding feature dimension (e.g. 768)
}

// NewAgeGenderModel loads the age/gender/voice-print model from modelPath. It
// reads the model metadata, routes the audio input and the three outputs by
// dimension, and creates the inference session. threads <= 0 uses the
// createSession default. The ONNX Runtime must be initialized before calling.
func NewAgeGenderModel(modelPath string, threads int) (*AgeGenderModel, error) {
	if modelPath == "" {
		return nil, ErrModelPathRequired
	}

	inputInfos, outputInfos, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("voicewatch: failed to load age/gender model metadata: %w", err)
	}

	inputName, err := routeAudioInput(inputInfos)
	if err != nil {
		return nil, err
	}

	ageIdx, genderIdx, embedIdx, embedDim, err := routeOutputs(outputInfos)
	if err != nil {
		return nil, err
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

	session, err := createSession(modelPath, []string{inputName}, outputNames, optsFn)
	if err != nil {
		return nil, err
	}

	return &AgeGenderModel{
		session:     session,
		inputName:   inputName,
		outputNames: outputNames,
		ageIdx:      ageIdx,
		genderIdx:   genderIdx,
		embedIdx:    embedIdx,
		embedDim:    embedDim,
	}, nil
}

// routeAudioInput selects the single float32 input as the audio waveform input.
// It returns an AgeGenderIOError when there is not exactly one float32 input.
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
		return "", &AgeGenderIOError{
			Reason: fmt.Sprintf("expected exactly one float32 input, found %d", count),
		}
	}
	return name, nil
}

// routeOutputs classifies the three model outputs by their trailing (feature)
// dimension: dim 1 -> age, dim 3 -> gender, the largest remaining dim ->
// embedding. It returns an AgeGenderIOError if any of the three cannot be
// unambiguously identified.
//
// Assumption: the age and gender feature dimensions are static (1 and 3); only
// the batch dimension may be dynamic. This holds for the audEERING export.
func routeOutputs(outputs []ort.InputOutputInfo) (ageIdx, genderIdx, embedIdx int, embedDim int64, err error) {
	ageIdx, genderIdx, embedIdx = -1, -1, -1
	embedDim = -1

	for i := range outputs {
		dims := outputs[i].Dimensions
		if len(dims) == 0 {
			continue
		}
		featDim := dims[len(dims)-1]
		switch featDim {
		case ageGenderAgeDim:
			if ageIdx != -1 {
				return -1, -1, -1, 0, &AgeGenderIOError{Reason: "multiple outputs with feature dim 1 (age is ambiguous)"}
			}
			ageIdx = i
		case ageGenderGenderDim:
			if genderIdx != -1 {
				return -1, -1, -1, 0, &AgeGenderIOError{Reason: "multiple outputs with feature dim 3 (gender is ambiguous)"}
			}
			genderIdx = i
		default:
			// Embedding is the largest remaining feature dimension.
			if featDim > embedDim {
				embedDim = featDim
				embedIdx = i
			}
		}
	}

	if ageIdx == -1 || genderIdx == -1 || embedIdx == -1 {
		return -1, -1, -1, 0, &AgeGenderIOError{
			Reason: fmt.Sprintf("could not route all outputs by dimension (age=%d gender=%d embedding=%d)", ageIdx, genderIdx, embedIdx),
		}
	}
	return ageIdx, genderIdx, embedIdx, embedDim, nil
}

// Infer runs one age/gender/voice-print inference over a 16 kHz mono waveform.
// It returns the raw age score (0..1), the gender logits ([child, female, male]),
// and the voice-print embedding. The returned slices are freshly allocated and
// owned by the caller. NOT goroutine-safe.
func (m *AgeGenderModel) Infer(samples []float32) (ageScore float32, genderLogits, embedding []float32, err error) {
	if m.session == nil {
		return 0, nil, nil, ErrSessionClosed
	}
	if len(samples) == 0 {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender inference requires non-empty samples")
	}

	normalized := normalizeWaveform(samples)

	inputTensor, err := ort.NewTensor(ort.NewShape(ageGenderBatch, int64(len(normalized))), normalized)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender input tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()

	ageTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(ageGenderBatch, ageGenderAgeDim))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender age tensor: %w", err)
	}
	defer func() { _ = ageTensor.Destroy() }()

	genderTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(ageGenderBatch, ageGenderGenderDim))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender gender tensor: %w", err)
	}
	defer func() { _ = genderTensor.Destroy() }()

	embedTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(ageGenderBatch, m.embedDim))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender embedding tensor: %w", err)
	}
	defer func() { _ = embedTensor.Destroy() }()

	outputs := make([]ort.Value, len(m.outputNames))
	outputs[m.ageIdx] = ageTensor
	outputs[m.genderIdx] = genderTensor
	outputs[m.embedIdx] = embedTensor

	if err := m.session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender inference failed: %w", err)
	}

	ageData := ageTensor.GetData()
	if len(ageData) == 0 {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender produced empty age output")
	}
	genderData := genderTensor.GetData()
	if len(genderData) < ageGenderGenderDim {
		return 0, nil, nil, fmt.Errorf("voicewatch: age/gender gender output too small: got %d, want %d", len(genderData), ageGenderGenderDim)
	}
	embedData := embedTensor.GetData()

	genderLogits = make([]float32, len(genderData))
	copy(genderLogits, genderData)
	embedding = make([]float32, len(embedData))
	copy(embedding, embedData)

	return ageData[0], genderLogits, embedding, nil
}

// normalizeWaveform applies Wav2Vec2 per-clip zero-mean, unit-variance
// normalization: (x - mean) / sqrt(variance + eps). It returns a new slice and
// does not mutate the input.
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
	denom := math.Sqrt(variance + ageGenderNormEps)

	for i, s := range samples {
		out[i] = float32((float64(s) - mean) / denom)
	}
	return out
}

// Close releases the ONNX session and associated resources.
func (m *AgeGenderModel) Close() error {
	if m.session != nil {
		err := m.session.Destroy()
		m.session = nil
		return err
	}
	return nil
}
