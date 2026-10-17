import numpy as np
from scipy import optimize

# first output marks "imports done"
print("ready: n-optimize")

res = optimize.minimize(optimize.rosen, np.full(20, 1.3), method="BFGS")
print(f"success={res.success} f={res.fun:.2e} nit={res.nit}")
