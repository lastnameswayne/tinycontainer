import numpy as np

# first output marks "imports done"
print("ready: n-fft")

x = np.random.default_rng(0).standard_normal(1 << 22)
spec = np.fft.rfft(x)
print(f"bins={spec.size} peak={np.abs(spec).max():.1f}")
