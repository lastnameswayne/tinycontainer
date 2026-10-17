import numpy as np
from scipy import sparse
from scipy.sparse import linalg as spla

# first output marks "imports done"
print("ready: n-sparse")

n = 200_000
A = sparse.diags([-1, 2.5, -1], [-1, 0, 1], shape=(n, n), format="csr")
b = np.ones(n)
x, info = spla.cg(A, b)
print(f"info={info} residual={np.linalg.norm(A @ x - b):.2e}")
