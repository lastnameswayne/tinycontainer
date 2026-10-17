import numpy as np
import statsmodels.api as sm

# first output marks "imports done"
print("ready: d-statsmodels")

rng = np.random.default_rng(0)
X = sm.add_constant(rng.standard_normal((5000, 4)))
y = X @ np.array([1.0, 2.0, -1.0, 0.5, 0.0]) + rng.standard_normal(5000)
fit = sm.OLS(y, X).fit()
print(f"r2={fit.rsquared:.4f} params={np.round(fit.params, 2).tolist()}")
