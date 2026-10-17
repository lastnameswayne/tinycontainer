import numpy as np
import xgboost as xgb

# first output marks "imports done"
print("ready: m-xgboost")

rng = np.random.default_rng(0)
X = rng.standard_normal((20_000, 30))
y = (X[:, 0] + X[:, 1] ** 2 > 1).astype(int)
bst = xgb.train({"objective": "binary:logistic", "nthread": 1, "max_depth": 4}, xgb.DMatrix(X, label=y), 50)
print(f"pred mean={bst.predict(xgb.DMatrix(X[:1000])).mean():.4f}")
