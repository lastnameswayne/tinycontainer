from sklearn.datasets import make_regression
from sklearn.ensemble import RandomForestRegressor

# first output marks "imports done"
print("ready: m-sklearn-forest")

X, y = make_regression(n_samples=5000, n_features=20, noise=0.5, random_state=0)
m = RandomForestRegressor(n_estimators=50, n_jobs=1, random_state=0).fit(X, y)
print(f"train r2={m.score(X, y):.4f}")
