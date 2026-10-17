import pandas as pd

print("ready: pandas", flush=True)

# kept outside /app and /opt: sway maps /app to the container root, and the worker's /opt is a local empty dir
df = pd.read_csv("/usr/local/share/tcdata/pandas-data.csv")
g = df.groupby("key").agg({"a": "mean", "b": "std", "c": "sum"})
print(f"rows={len(df)} groups={len(g)} a_mean={g['a'].mean():.4f}")
