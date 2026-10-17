import io
import numpy as np
import pandas as pd
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

# first output marks "imports done"
print("ready: d-matplotlib")

rng = np.random.default_rng(0)
df = pd.DataFrame({"day": np.arange(365), "sales": rng.normal(100, 15, 365).cumsum()})
fig, ax = plt.subplots()
df.plot(x="day", y="sales", ax=ax)
buf = io.BytesIO()
fig.savefig(buf, format="png")
print(f"png bytes={len(buf.getvalue())}")
