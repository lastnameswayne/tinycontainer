import numpy as np

# first output marks "imports done"; flush so it isn't held in the pipe buffer
print("ready: numpy", flush=True)

a = np.random.default_rng(0).standard_normal((500, 500))
inv = np.linalg.inv(a)
print(f"inv: {inv.shape}, max|A@inv - I| = {np.abs(a @ inv - np.eye(500)).max():.2e}")
