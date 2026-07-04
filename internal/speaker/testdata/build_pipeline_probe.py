"""Build a minimal, valid 2-class 'gender' ONNX model for the VoiceWatch speaker
harness. It is NOT a real classifier — it exercises the Go ONNX-inference
pipeline (SingleOutputAudioModel: single float32 waveform in [batch, samples] ->
single float32 output vector [batch, 2]) end to end against real ONNX Runtime.

Graph: waveform[batch, samples] --ReduceMean(axis=1)--> mean[batch,1]
       --MatMul(W[1,2])--> logits[batch,2]
"""

import numpy as np
import onnx
from onnx import TensorProto, helper, numpy_helper

# W maps the clip mean-amplitude to two class logits [male, female].
# Arbitrary but deterministic; the smoke test only needs a valid concrete label.
W = numpy_helper.from_array(
    np.array([[1.0, -1.0]], dtype=np.float32), name="W"
)

reduce_mean = helper.make_node(
    "ReduceMean", inputs=["waveform"], outputs=["mean"], axes=[1], keepdims=1
)
matmul = helper.make_node("MatMul", inputs=["mean", "W"], outputs=["logits"])

graph = helper.make_graph(
    nodes=[reduce_mean, matmul],
    name="voice_gender_min",
    inputs=[
        helper.make_tensor_value_info(
            "waveform", TensorProto.FLOAT, ["batch", "samples"]
        )
    ],
    outputs=[
        helper.make_tensor_value_info("logits", TensorProto.FLOAT, ["batch", 2])
    ],
    initializer=[W],
)

# opset 13 keeps ReduceMean `axes` as an attribute (opset 18 moved it to an input).
model = helper.make_model(
    graph, opset_imports=[helper.make_opsetid("", 13)], ir_version=9
)
model.doc_string = "Minimal 2-class gender-shaped model for pipeline validation only"
onnx.checker.check_model(model)

out_path = "pipeline_probe_2class.onnx"
onnx.save(model, out_path)
print("wrote", out_path)

# Sanity: shapes
print("inputs:", [(i.name) for i in model.graph.input])
print("outputs:", [(o.name) for o in model.graph.output])
