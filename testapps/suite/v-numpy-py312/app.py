import numpy as np

# first output marks "imports done"
print("ready: v-numpy-py312")

a = np.random.default_rng(0).standard_normal((500, 500))
print(f"det sign={np.sign(np.linalg.det(a))}")
