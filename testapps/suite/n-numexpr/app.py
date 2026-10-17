import numpy as np
import numexpr as ne

# first output marks "imports done"
print("ready: n-numexpr")

rng = np.random.default_rng(0)
a, b = rng.standard_normal(5_000_000), rng.standard_normal(5_000_000)
r = ne.evaluate("sin(a) ** 2 + cos(b) ** 2 + 2 * a * b")
print(f"mean={r.mean():.4f}")
