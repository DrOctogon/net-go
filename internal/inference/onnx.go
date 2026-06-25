package inference

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/tphakala/voicewatch/internal/inference/onnx"
	ortlib "github.com/yalue/onnxruntime_go"
)

var (
	ortInitMu      sync.Mutex
	ortInitialized bool
)

// IsORTInitialized reports whether the ONNX Runtime has been successfully initialized. Thread-safe.
func IsORTInitialized() bool {
	ortInitMu.Lock()
	defer ortInitMu.Unlock()
	return ortInitialized
}

// ONNXClassifierOptions configures the ONNX species classifier.
type ONNXClassifierOptions struct {
	// Labels is the species label list. Required.
	Labels []string
	// Threads is the number of CPU threads for ONNX inference. 0 = use ONNX defaults.
	Threads int
	// SkipLabelValidation disables the label-count-vs-model-output check.
	// Use when the model is loaded only for embedding extraction and the
	// caller's label list may not match the model's logits dimension.
	SkipLabelValidation bool
}

// onnxClassifier implements Classifier using an ONNX Runtime session.
type onnxClassifier struct {
	classifier *ort.Classifier
	numSpecies int
}

// NewONNXClassifier creates a Classifier backed by an ONNX Runtime model.
// The ONNX Runtime must be initialized via InitONNXRuntime before calling this.
func NewONNXClassifier(modelPath string, opts ONNXClassifierOptions) (Classifier, error) {
	if len(opts.Labels) == 0 {
		return nil, fmt.Errorf("ONNX classifier requires labels")
	}

	classifierOpts := []ort.ClassifierOption{
		ort.WithLabels(opts.Labels),
		ort.WithTopK(0),          // We handle topK in BirdNET-Go's post-processing
		ort.WithMinConfidence(0), // No filtering, return all raw scores
	}
	if opts.SkipLabelValidation {
		classifierOpts = append(classifierOpts, ort.WithSkipLabelValidation())
	}
	threads := opts.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	var configErr error
	classifierOpts = append(classifierOpts, ort.WithSessionOptions(func(so *ortlib.SessionOptions) {
		if err := so.SetIntraOpNumThreads(threads); err != nil && configErr == nil {
			configErr = fmt.Errorf("failed to set IntraOpNumThreads to %d: %w", threads, err)
		}
		if err := so.SetInterOpNumThreads(threads); err != nil && configErr == nil {
			configErr = fmt.Errorf("failed to set InterOpNumThreads to %d: %w", threads, err)
		}
	}))
	classifier, err := ort.NewClassifier(modelPath, classifierOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX classifier: %w", err)
	}
	if configErr != nil {
		_ = classifier.Close()
		return nil, fmt.Errorf("failed to configure ONNX session options: %w", configErr)
	}

	return &onnxClassifier{
		classifier: classifier,
		numSpecies: len(opts.Labels),
	}, nil
}

// SileroVADOptions configures the Silero VAD speech detector.
type SileroVADOptions struct {
	// FrameSize is the per-inference window in samples (512 at 16 kHz). Required.
	FrameSize int
	// SampleRate is the value fed to the model's "sr" input (e.g. 16000). Required.
	SampleRate int64
	// Threads is the number of CPU threads for inference. 0 = ONNX defaults.
	Threads int
}

// onnxSilero adapts an *onnx.SileroVAD to the Classifier interface. Predict
// returns one speech probability per analysis frame (not per species); the
// human voice model collapses these into its single class downstream.
type onnxSilero struct {
	vad *ort.SileroVAD
}

// NewSileroVAD creates a Classifier backed by a Silero VAD ONNX model. The ONNX
// Runtime must be initialized via InitONNXRuntime before calling this.
func NewSileroVAD(modelPath string, opts SileroVADOptions) (Classifier, error) {
	vad, err := ort.NewSileroVAD(modelPath, opts.FrameSize, opts.SampleRate, opts.Threads)
	if err != nil {
		return nil, fmt.Errorf("failed to create Silero VAD: %w", err)
	}
	return &onnxSilero{vad: vad}, nil
}

// Predict runs the VAD over the clip, returning one speech probability per frame.
func (c *onnxSilero) Predict(samples []float32) ([]float32, error) {
	if c.vad == nil {
		return nil, ort.ErrSessionClosed
	}
	return c.vad.Predict(samples)
}

// NumSpecies returns 1: the human voice model has a single output class.
func (c *onnxSilero) NumSpecies() int { return 1 }

// Close releases the VAD session resources.
func (c *onnxSilero) Close() {
	if c.vad != nil {
		_ = c.vad.Close()
		c.vad = nil
	}
}

// Predict runs ONNX inference, returning raw logits (pre-activation).
func (c *onnxClassifier) Predict(samples []float32) ([]float32, error) {
	if c.classifier == nil {
		return nil, ort.ErrSessionClosed
	}
	return c.classifier.PredictRaw(samples)
}

// PredictWithEmbeddings runs ONNX inference, returning both raw logits and embedding vector.
func (c *onnxClassifier) PredictWithEmbeddings(samples []float32) (logits, embeddings []float32, err error) {
	if c.classifier == nil {
		return nil, nil, ort.ErrSessionClosed
	}
	return c.classifier.PredictRawWithEmbeddings(samples)
}

// NumSpecies returns the number of species in the model output.
func (c *onnxClassifier) NumSpecies() int {
	return c.numSpecies
}

// Close releases the ONNX session resources.
func (c *onnxClassifier) Close() {
	if c.classifier != nil {
		_ = c.classifier.Close()
		c.classifier = nil
	}
}

// ONNXCustomClassifierOptions configures the ONNX custom classifier.
type ONNXCustomClassifierOptions struct {
	Labels     []string // Provide labels directly (takes priority over LabelsPath)
	LabelsPath string   // Load labels from file (text, CSV, or JSON)
	Threads    int
}

type onnxCustomClassifier struct {
	classifier *ort.CustomClassifier
}

// NewONNXCustomClassifier creates a CustomClassifier backed by an ONNX Runtime model.
func NewONNXCustomClassifier(modelPath string, opts ONNXCustomClassifierOptions) (CustomClassifier, error) {
	builder := ort.NewCustomClassifierBuilder().
		ModelPath(modelPath).
		TopK(0).
		MinConfidence(0)

	switch {
	case len(opts.Labels) > 0:
		builder = builder.Labels(opts.Labels)
	case opts.LabelsPath != "":
		builder = builder.LabelsPath(opts.LabelsPath)
	default:
		return nil, fmt.Errorf("ONNX custom classifier requires labels or labels path")
	}

	threads := opts.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	var configErr error
	builder = builder.SessionOptions(func(so *ortlib.SessionOptions) {
		if err := so.SetIntraOpNumThreads(threads); err != nil && configErr == nil {
			configErr = fmt.Errorf("failed to set IntraOpNumThreads to %d: %w", threads, err)
		}
		if err := so.SetInterOpNumThreads(threads); err != nil && configErr == nil {
			configErr = fmt.Errorf("failed to set InterOpNumThreads to %d: %w", threads, err)
		}
	})

	cc, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX custom classifier: %w", err)
	}
	if configErr != nil {
		_ = cc.Close()
		return nil, fmt.Errorf("failed to configure ONNX custom classifier session: %w", configErr)
	}

	return &onnxCustomClassifier{
		classifier: cc,
	}, nil
}

// PredictEmbedding runs inference on an embedding vector.
func (c *onnxCustomClassifier) PredictEmbedding(embeddings []float32) ([]float32, error) {
	if c.classifier == nil {
		return nil, ort.ErrSessionClosed
	}
	return c.classifier.PredictRaw(embeddings)
}

// NumClasses returns the number of output classes.
func (c *onnxCustomClassifier) NumClasses() int {
	if c.classifier == nil {
		return 0
	}
	return c.classifier.NumClasses()
}

// InputDim returns the embedding vector length the classifier expects as input.
func (c *onnxCustomClassifier) InputDim() int {
	if c.classifier == nil {
		return 0
	}
	return c.classifier.InputDim()
}

// Labels returns the classification labels.
func (c *onnxCustomClassifier) Labels() []string {
	if c.classifier == nil {
		return nil
	}
	return c.classifier.Labels()
}

// Close releases the ONNX custom classifier session.
func (c *onnxCustomClassifier) Close() {
	if c.classifier != nil {
		_ = c.classifier.Close()
		c.classifier = nil
	}
}

// InitONNXRuntime initializes the ONNX Runtime with the given shared library path.
// When libraryPath is empty, searches standard system library paths for
// libonnxruntime.so (Linux) or onnxruntime.dll (Windows).
// Safe to call multiple times; skips if already initialized successfully.
// On failure, allows retry with a corrected path (supports hot-reload recovery).
func InitONNXRuntime(libraryPath string) (err error) {
	ortInitMu.Lock()
	defer ortInitMu.Unlock()

	if ortInitialized {
		return nil
	}

	if libraryPath == "" {
		libraryPath = findONNXRuntimeLibrary()
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed to initialize ONNX Runtime: %v; install guide: %s", r, ORTInstallGuideURL)
		}
	}()

	ort.MustInitORT(libraryPath)
	ortInitialized = true
	return nil
}

// findONNXRuntimeLibrary searches standard system paths for the ONNX Runtime
// shared library. Returns the first path found, or "onnxruntime" as a
// fallback for dlopen's default search.
func findONNXRuntimeLibrary() string {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{"onnxruntime.dll"}
		if exePath, err := os.Executable(); err == nil {
			candidates = append([]string{filepath.Join(filepath.Dir(exePath), "onnxruntime.dll")}, candidates...)
		}
		for i, c := range candidates {
			if abs, err := filepath.Abs(c); err == nil {
				candidates[i] = abs
			}
		}
	case "darwin":
		candidates = []string{
			"/opt/homebrew/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
			"libonnxruntime.dylib",
		}
	default:
		candidates = []string{
			"/usr/lib/libonnxruntime.so",
			"/usr/local/lib/libonnxruntime.so",
			"/usr/lib/aarch64-linux-gnu/libonnxruntime.so",
			"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
			"libonnxruntime.so",
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if runtime.GOOS == "darwin" {
		return "libonnxruntime.dylib"
	}
	return "onnxruntime"
}

// DestroyONNXRuntime tears down the ONNX Runtime environment.
// Resets initialization state so InitONNXRuntime can be called again. It is a
// no-op (returns nil) when the runtime was never initialized, mirroring
// DestroyOpenVINO, so a shutdown teardown can call it unconditionally without
// ort.DestroyORT reporting "InitializeRuntime has not been called".
func DestroyONNXRuntime() error {
	ortInitMu.Lock()
	defer ortInitMu.Unlock()
	if !ortInitialized {
		return nil
	}
	if err := ort.DestroyORT(); err != nil {
		return err
	}
	ortInitialized = false
	return nil
}
