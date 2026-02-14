import numpy as np
from scipy import linalg, stats

def main():
    print("scipy test")

    a = np.random.randn(100, 100)
    b = np.random.randn(100, 100)
    c = a @ b
    print(f"matmul: {c.shape}")

    u, s, vh = linalg.svd(a)
    print(f"svd: u={u.shape}, s={s.shape}")

    x = np.random.randn(1000)
    mean, var = stats.norm.fit(x)
    print(f"fitted normal: mean={mean:.3f}, var={var:.3f}")

if __name__ == "__main__":
    main()
