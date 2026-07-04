# Speaker test fixtures

## `pipeline_probe_2class.onnx`

A **synthetic** ONNX model used only to exercise the speaker gender inference
pipeline (`internal/inference/onnx.SingleOutputAudioModel` + `mapGender2Class`)
end to end against real ONNX Runtime. **It is not a gender classifier** and has
no accuracy — do not ship it or treat its output as meaningful.

Graph: `waveform[batch, samples] --ReduceMean(axis=1)--> [batch, 1] --MatMul([[1, -1]])--> logits[batch, 2]`.
This projects a clip's mean amplitude to two class logits `[mean, -mean]`, which
makes the `[male, female]` mapping order and the softmax confidence bound
deterministic: a positive-mean waveform maps to `male`, a negative-mean waveform
to `female`.

Consumed by `TestGenderPipeline_SyntheticProbe` (skips when the ONNX Runtime
shared library is absent). For a REAL gender model + accuracy check, use
`TestONNXGenderModelSmoke` with `VW_ORT_LIB` + `VW_SPEAKER_GENDER_MODEL`.

Regenerate with:

```sh
python3 build_pipeline_probe.py   # needs the `onnx` package (no torch required)
```
