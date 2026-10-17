import os

import numpy as np
from onnx import TensorProto, helper, save
rng = np.random.default_rng(0)
w1 = helper.make_tensor("w1", TensorProto.FLOAT, [128, 512], rng.standard_normal((128, 512)).astype(np.float32).ravel())
w2 = helper.make_tensor("w2", TensorProto.FLOAT, [512, 10], rng.standard_normal((512, 10)).astype(np.float32).ravel())
nodes = [helper.make_node("MatMul", ["x", "w1"], ["h"]), helper.make_node("Relu", ["h"], ["r"]),
         helper.make_node("MatMul", ["r", "w2"], ["y"])]
graph = helper.make_graph(nodes, "mlp", [helper.make_tensor_value_info("x", TensorProto.FLOAT, [None, 128])],
                          [helper.make_tensor_value_info("y", TensorProto.FLOAT, [None, 10])], [w1, w2])
os.makedirs("/usr/local/share/tcdata", exist_ok=True)
save(helper.make_model(graph, opset_imports=[helper.make_opsetid("", 17)]), "/usr/local/share/tcdata/dl-onnxruntime-model.onnx")
