package onnx

// lastDim returns the last element of a shape slice as an int, or 0 if empty.
func lastDim(shape []int64) int {
	if len(shape) == 0 {
		return 0
	}
	return int(shape[len(shape)-1])
}
