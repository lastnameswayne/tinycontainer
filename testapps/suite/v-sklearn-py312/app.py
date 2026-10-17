from sklearn.cluster import KMeans
from sklearn.datasets import make_blobs

# first output marks "imports done"
print("ready: v-sklearn-py312")

X, _ = make_blobs(n_samples=20_000, centers=8, random_state=0)
print(f"inertia={KMeans(n_clusters=8, n_init=3, random_state=0).fit(X).inertia_:.1f}")
