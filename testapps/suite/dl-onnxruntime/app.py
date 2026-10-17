import numpy as np
import onnxruntime as ort

# first output marks "imports done"
print("ready: dl-onnxruntime")

sess = ort.InferenceSession("/usr/local/share/tcdata/dl-onnxruntime-model.onnx", providers=["CPUExecutionProvider"])
x = np.random.default_rng(0).standard_normal((256, 128)).astype(np.float32)
(y,) = sess.run(None, {"x": x})
print(f"out={y.shape} mean={y.mean():.4f}")
