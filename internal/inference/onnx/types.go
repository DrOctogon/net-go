package onnx

// Prediction represents a single classification prediction with confidence score.
type Prediction struct {
	Species    string
	Confidence float32
	Index      int
}
