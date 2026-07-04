# Classifier Package

The `classifier` package hosts the audio-classification model that powers
VoiceWatch detection. It provides a generic, backend-agnostic single-model host
plus the shipped model implementation.

> **Human-voice pivot:** This package was the BirdNET-Go multi-model
> bird-identification core (primary BirdNET + secondary Perch/Bat, geographic
> range-filter coordination, eBird taxonomy, name-resolver chain, model
> gallery). Those were removed in the pivot and replaced by a thin single-model
> host that runs an embedded [Silero VAD](https://github.com/snakers4/silero-vad)
> speech detector. The upstream BirdNET-Go heritage is retained in the project's
> identifiers and attribution.

## Package Overview

- **Generic model host** — a thin orchestrator that owns one active
  `ModelInstance` and serializes inference against it.
- **Single-model registry** — the supported-model registry, currently just the
  Silero VAD "Human Voice" model.
- **Backend-agnostic interfaces** — `ModelInstance` / `ModelSpec` abstractions
  that keep the host independent of the inference backend (TFLite, ONNX Runtime,
  OpenVINO).
- **Results queue** — an ownership-transfer channel for passing inference
  results to downstream consumers without deep-copying PCM audio.

## Core Components

### Orchestrator (`orchestrator.go`)

Owns the single loaded model and serializes inference against it. The concrete
`ModelInstance` (`humanvoice.Model`) is constructed in the analysis layer and
injected via `NewOrchestrator`, which avoids a `humanvoice` → `classifier`
import cycle. Reports the loaded model's device via `RuntimeInfo()`.

### Model Registry (`model_registry.go`)

The single source of truth for supported models — reduced to the Silero VAD
"Human Voice" model. Defines the inference-backend identifiers
(`BackendTFLite` / `BackendONNX` are static model file-type metadata;
`BackendOpenVINO` is a live execution provider reported at runtime) and the
model's quantization precision, which is orthogonal to the backend.

### Model Interfaces (`model.go`)

`ModelInstance` and `ModelSpec` define the multi-model abstraction. `ModelSpec`
describes a model's fixed audio requirements (sample rate, clip length); overlap
is intentionally excluded — it comes from the false-positive-filter
configuration. Device names (`deviceCPU`, or a concrete OpenVINO device) are
reported by `ModelInstance.RuntimeInfo()`; the orchestrator returns
`deviceUnknown` when no model is loaded.

### Results Queue (`queue.go`)

`Results` carries a clip's inference output and metadata. `ResultsQueue` is a
channel with ownership-transfer semantics: once a `Results` is sent, the sender
must not modify it, so the pipeline can hand off PCM audio without a deep copy.
`Results.Copy()` provides an explicit deep copy when independent ownership is
needed.

### Human Voice Model (`humanvoice/`)

The shipped `ModelInstance`: a Silero VAD ONNX model that produces per-frame
speech probabilities and aggregates each clip into a single "Human Voice"
detection. The model file is embedded in the binary (`embed.go`) and written to
disk at runtime on first use. See `humanvoice/humanvoice.go`.

## Thread Safety

The orchestrator serializes inference against the single loaded model, and the
results queue is a channel — both are safe for concurrent use by the audio
pipeline and its consumers.
