"""Build minimal, valid SYNTHETIC ONNX probe models for the VoiceWatch speaker
attribute pipelines. They are NOT real classifiers/extractors — each one
exercises the Go ONNX-inference path end to end against real ONNX Runtime
(internal/inference/onnx.SingleOutputAudioModel: a single float32 waveform input
[batch, samples] -> a single float32 output vector), plus the attribute mapping.

Each probe reduces the clip to its mean amplitude, then projects that mean to the
output shape the corresponding mapping expects, so outputs are deterministic
functions of the input mean:

  gender  (pipeline_probe_2class.onnx):    mean -> logits[batch, 2] = [mean, -mean]
                                           (mapGender2Class: +mean -> male, -mean -> female)
  age     (pipeline_probe_age.onnx):       mean -> score[batch, 1]  = mean
                                           (mapAge: years = score*100; 0.05 -> child, 0.35 -> adult)
  vprint  (pipeline_probe_embedding.onnx): mean -> embedding[batch, 8] = mean*[1..8]

Requires only the `onnx` package (no torch). Regenerate with:
    python3 build_pipeline_probe.py
"""

import numpy as np
import onnx
from onnx import TensorProto, helper, numpy_helper


def _reduce_mean_node():
    # opset 13 keeps ReduceMean `axes` as an attribute (opset 18 moved it to an input).
    return helper.make_node(
        "ReduceMean", inputs=["waveform"], outputs=["mean"], axes=[1], keepdims=1
    )


def _waveform_input():
    return helper.make_tensor_value_info(
        "waveform", TensorProto.FLOAT, ["batch", "samples"]
    )


def _save(graph, path: str, doc: str) -> None:
    model = helper.make_model(
        graph, opset_imports=[helper.make_opsetid("", 13)], ir_version=9
    )
    model.doc_string = doc
    onnx.checker.check_model(model)
    onnx.save(model, path)
    print(
        "wrote",
        path,
        "inputs",
        [i.name for i in model.graph.input],
        "outputs",
        [o.name for o in model.graph.output],
    )


def build_gender() -> None:
    # mean -> [mean, -mean]: mapGender2Class argmax 0 (male) for +mean, 1 (female) for -mean.
    w = numpy_helper.from_array(np.array([[1.0, -1.0]], dtype=np.float32), name="Wg")
    graph = helper.make_graph(
        nodes=[
            _reduce_mean_node(),
            helper.make_node("MatMul", inputs=["mean", "Wg"], outputs=["logits"]),
        ],
        name="speaker_gender_probe",
        inputs=[_waveform_input()],
        outputs=[helper.make_tensor_value_info("logits", TensorProto.FLOAT, ["batch", 2])],
        initializer=[w],
    )
    _save(graph, "pipeline_probe_2class.onnx", "synthetic 2-class gender pipeline probe")


def build_age() -> None:
    # mean -> score[batch, 1]; mapAge reads out[0]. score*100 = years.
    graph = helper.make_graph(
        nodes=[
            helper.make_node(
                "ReduceMean", inputs=["waveform"], outputs=["score"], axes=[1], keepdims=1
            )
        ],
        name="speaker_age_probe",
        inputs=[_waveform_input()],
        outputs=[helper.make_tensor_value_info("score", TensorProto.FLOAT, ["batch", 1])],
        initializer=[],
    )
    _save(graph, "pipeline_probe_age.onnx", "synthetic age-score pipeline probe")


def build_embedding() -> None:
    # mean -> embedding[batch, 8] = mean*[1..8]; exercises the voice-print path + Cosine.
    w = numpy_helper.from_array(
        np.array([[1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0]], dtype=np.float32),
        name="We",
    )
    graph = helper.make_graph(
        nodes=[
            _reduce_mean_node(),
            helper.make_node("MatMul", inputs=["mean", "We"], outputs=["embedding"]),
        ],
        name="speaker_voiceprint_probe",
        inputs=[_waveform_input()],
        outputs=[helper.make_tensor_value_info("embedding", TensorProto.FLOAT, ["batch", 8])],
        initializer=[w],
    )
    _save(graph, "pipeline_probe_embedding.onnx", "synthetic voice-print embedding pipeline probe")


if __name__ == "__main__":
    build_gender()
    build_age()
    build_embedding()
