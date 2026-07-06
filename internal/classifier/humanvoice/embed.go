package humanvoice

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/tphakala/voicewatch/internal/errors"
)

// EmbeddedModelName is the filename the embedded Silero VAD model is written to.
const EmbeddedModelName = "silero_vad.onnx"

// embeddedModel is the Silero VAD v5 ONNX model shipped with the binary. It is
// extracted to the models directory on first run so the ONNX Runtime can load it
// by path and operators can inspect or replace it.
//
//go:embed data/silero_vad.onnx
var embeddedModel []byte

// WriteEmbeddedModel writes the embedded Silero VAD model into dir and returns
// the full path. It is a no-op (returns the existing path) when a file of the
// expected size is already present, so it is cheap to call on every startup.
func WriteEmbeddedModel(dir string) (string, error) {
	if dir == "" {
		return "", errors.Newf("humanvoice: empty models directory").
			Category(errors.CategoryValidation).
			Build()
	}
	if len(embeddedModel) == 0 {
		return "", errors.Newf("humanvoice: embedded model is empty").
			Category(errors.CategoryModelInit).
			Build()
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", errors.New(err).
			Category(errors.CategoryFileIO).
			Context("operation", "create_models_dir").
			Context("dir", dir).
			Build()
	}

	path := filepath.Join(dir, EmbeddedModelName)
	if info, statErr := os.Stat(path); statErr == nil && info.Size() == int64(len(embeddedModel)) {
		return path, nil
	}

	if err := os.WriteFile(path, embeddedModel, 0o644); err != nil {
		return "", errors.New(err).
			Category(errors.CategoryFileIO).
			Context("operation", "write_embedded_model").
			Context("path", path).
			Build()
	}
	return path, nil
}
