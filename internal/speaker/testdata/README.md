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

## Real gender model (operator-supplied)

VoiceWatch ships no speaker model. `export_gender_model.py` produces a real,
MIT-licensed gender `.onnx` from
[JaesungHuh/voice-gender-classifier](https://huggingface.co/JaesungHuh/voice-gender-classifier)
(ECAPA-TDNN) that plugs straight into `SingleOutputAudioModel` + `mapGender2Class`.
The network does preemphasis + mel + log + mean-norm **in-graph** on a raw 16 kHz
mono waveform, so the export takes a raw waveform (`waveform[batch, samples]` →
`logits[batch, 2]`, order `[male, female]`) — matching the factory's
`normalize=false` wiring.

```sh
pip install torch torchaudio safetensors onnx onnxscript huggingface_hub
python3 export_gender_model.py            # writes ./voice_gender_classifier.onnx
```

Validated end to end through the Go pipeline via `TestONNXGenderModelSmoke`
(ONNX Runtime 1.25.1): the model's own `example1.wav` → `female` (conf 0.9896)
and `example2.wav` → `male` (conf 0.9914). The `.onnx` itself is **not** vendored
— operators supply it.
