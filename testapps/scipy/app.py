import numpy as np
from scipy import linalg

print("ready: scipy", flush=True)

a = np.random.randn(100, 100)
u, s, vh = linalg.svd(a)
print(f"svd: u={u.shape}, s={s.shape}, vh={vh.shape}")
