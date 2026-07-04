# Speaker test fixtures

`pipeline_probe_*.onnx` are **synthetic** ONNX models used only to exercise the
speaker attribute inference pipelines (`internal/inference/onnx.SingleOutputAudioModel`
+ the `mapGender`/`mapAge` mappings and the voice-print `Cosine`) end to end
against real ONNX Runtime. **They are not real classifiers/extractors** and have
no accuracy — do not ship them or treat their output as meaningful.

Each probe reduces a clip to its mean amplitude and projects that mean to the
output shape the mapping expects, so results are deterministic functions of the
input mean:

| File | Output | Mapping behaviour |
| --- | --- | --- |
| `pipeline_probe_2class.onnx` | `logits[batch, 2]` = `[mean, -mean]` | `mapGender2Class`: +mean → male, −mean → female |
| `pipeline_probe_age.onnx` | `score[batch, 1]` = `mean` | `mapAge`: years = score·100; 0.05 → child, 0.35 → adult |
| `pipeline_probe_embedding.onnx` | `embedding[batch, 8]` = `mean·[1..8]` | surfaced as `Attributes.Embedding`; `Cosine(e,e)` = 1 |

Consumed by `TestGenderPipeline_SyntheticProbe`, `TestAgePipeline_SyntheticProbe`,
and `TestVoicePrintPipeline_SyntheticProbe` (each skips when the ONNX Runtime
shared library is absent). For a REAL gender model + accuracy check, use
`TestONNXGenderModelSmoke` with `VW_ORT_LIB` + `VW_SPEAKER_GENDER_MODEL`.

Regenerate with:

```sh
python3 build_pipeline_probe.py   # needs the `onnx` package (no torch required)
```
