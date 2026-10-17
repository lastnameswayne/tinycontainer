from sklearn.datasets import make_classification
from sklearn.linear_model import LogisticRegression
from sklearn.model_selection import cross_val_score

# first output marks "imports done"
print("ready: m-sklearn-logreg")

X, y = make_classification(n_samples=20_000, n_features=40, random_state=0)
print(f"cv accuracy={cross_val_score(LogisticRegression(max_iter=500), X, y, cv=3).mean():.4f}")
