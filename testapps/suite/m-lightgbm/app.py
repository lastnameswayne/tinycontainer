import numpy as np
import lightgbm as lgb

# first output marks "imports done"
print("ready: m-lightgbm")

rng = np.random.default_rng(0)
X = rng.standard_normal((20_000, 30))
y = X[:, 0] * 2 + rng.standard_normal(20_000)
m = lgb.train({"objective": "regression", "num_threads": 1, "verbose": -1}, lgb.Dataset(X, y), 50)
print(f"pred[0:3]={np.round(m.predict(X[:3]), 3).tolist()}")
