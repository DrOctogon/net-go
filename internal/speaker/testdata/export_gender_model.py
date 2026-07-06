#!/usr/bin/env python3
"""Export the JaesungHuh/voice-gender-classifier (ECAPA-TDNN) to ONNX for VoiceWatch.

VoiceWatch ships NO speaker model — operators supply their own `.onnx`. This
script produces a real, MIT-licensed gender `.onnx` that plugs straight into the
`internal/inference/onnx.SingleOutputAudioModel` pipeline and the `mapGender2Class`
mapping, and is validated end to end by `TestONNXGenderModelSmoke`.

Model card: https://huggingface.co/JaesungHuh/voice-gender-classifier (MIT).
The ECAPA_gender network does preemphasis + mel + log + mean-norm IN-GRAPH on a
RAW 16 kHz mono waveform, so the exported graph takes a raw waveform directly —
matching the factory's `normalize=false` wiring (no external normalisation).

    Input:  waveform  float32 [batch, samples]   (raw 16 kHz mono)
    Output: logits    float32 [batch, 2]          order = [male, female]
            (pred2gender = {0: male, 1: female})

Validated (ONNX Runtime 1.25.1, this export):
    example1.wav -> female  (conf 0.9896)
    example2.wav -> male    (conf 0.9914)

Requires (host-side, NOT a VoiceWatch build dependency):
    pip install torch torchaudio safetensors onnx onnxscript huggingface_hub

Usage:
    python3 export_gender_model.py               # writes ./voice_gender_classifier.onnx
    python3 export_gender_model.py out.onnx

Then wire it into the Go smoke test:
    VW_ORT_LIB=/opt/homebrew/lib/libonnxruntime.dylib \
    VW_SPEAKER_GENDER_MODEL=$PWD/voice_gender_classifier.onnx \
    go test -run TestONNXGenderModelSmoke -tags "notflite skipfrontend" ./internal/speaker/
"""

import os
import sys
import types
import urllib.request

import torch
import torch.nn.functional as F
import torchaudio

REPO = "JaesungHuh/voice-gender-classifier"
MODEL_PY_URL = "https://raw.githubusercontent.com/JaesungHuh/voice-gender-classifier/main/model.py"


def _load_model_class(workdir: str):
    """Fetch the ECAPA_gender class definition (lives in the GitHub repo, not on HF)."""
    model_py = os.path.join(workdir, "model.py")
    if not os.path.exists(model_py):
        urllib.request.urlretrieve(MODEL_PY_URL, model_py)  # noqa: S310 - pinned raw URL
    sys.path.insert(0, workdir)
    from model import ECAPA_gender  # type: ignore

    return ECAPA_gender


def main() -> None:
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.getcwd(), "voice_gender_classifier.onnx")
    workdir = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".gender_export_cache")
    os.makedirs(workdir, exist_ok=True)

    ECAPA_gender = _load_model_class(workdir)
    model = ECAPA_gender.from_pretrained(REPO).eval()
    print("pred2gender:", getattr(model, "pred2gender", "??"))

    # The stock logtorchfbank() builds a torchaudio MelSpectrogram INSIDE forward();
    # its mel-filterbank construction has a data-dependent branch that torch.export
    # cannot trace. Build the transform once here so that branch runs eagerly, then
    # rebind the method to reuse it — numerically identical, but export-friendly.
    mel = torchaudio.transforms.MelSpectrogram(
        sample_rate=16000, n_fft=512, win_length=400, hop_length=160,
        f_min=20, f_max=7600, window_fn=torch.hamming_window, n_mels=80,
    ).eval()
    model.add_module("_mel_cached", mel)

    def logtorchfbank(self, x):
        flipped = torch.tensor([-0.97, 1.0]).unsqueeze(0).unsqueeze(0)
        x = x.unsqueeze(1)
        x = F.pad(x, (1, 0), "reflect")
        x = F.conv1d(x, flipped).squeeze(1)
        x = self._mel_cached(x) + 1e-6
        x = x.log()
        return x - torch.mean(x, dim=-1, keepdim=True)

    model.logtorchfbank = types.MethodType(logtorchfbank, model)

    dummy = torch.randn(1, 16000 * 3)  # 3 s @ 16 kHz
    with torch.no_grad():
        shape = tuple(model(dummy).shape)
    assert shape == (1, 2), f"unexpected forward output shape {shape}"

    torch.onnx.export(
        model, dummy, out,
        input_names=["waveform"], output_names=["logits"],
        dynamic_axes={"waveform": {0: "batch", 1: "samples"}, "logits": {0: "batch"}},
        opset_version=17, do_constant_folding=True, dynamo=True,
    )
    print("WROTE", out, os.path.getsize(out), "bytes")


if __name__ == "__main__":
    main()
