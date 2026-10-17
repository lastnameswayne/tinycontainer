import numpy as np
from numba import njit

# first output marks "imports done"
print("ready: n-numba")

@njit(cache=False)
def pairwise(x):
    n = x.shape[0]
    total = 0.0
    for i in range(n):
        for j in range(i + 1, n):
            d = x[i] - x[j]
            total += np.sqrt((d * d).sum())
    return total

x = np.random.default_rng(0).standard_normal((2000, 3))
print(f"sum of distances={pairwise(x):.1f}")
