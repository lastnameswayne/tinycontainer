import numpy as np
import pandas as pd

# first output marks "imports done"
print("ready: v-pandas-py311")

rng = np.random.default_rng(0)
df = pd.DataFrame({"k": rng.integers(0, 50, 200_000), "v": rng.standard_normal(200_000)})
print(f"groups={df.groupby('k').v.mean().size}")
