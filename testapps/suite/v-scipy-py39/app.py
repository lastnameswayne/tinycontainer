import numpy as np
from scipy import linalg

# first output marks "imports done"
print("ready: v-scipy-py39")

a = np.random.default_rng(0).standard_normal((300, 300))
print(f"cond={np.linalg.cond(linalg.expm(a / 50)):.2f}")
