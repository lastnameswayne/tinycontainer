import polars as pl

# first output marks "imports done"
print("ready: d-polars")

df = pl.DataFrame({"k": [i % 1000 for i in range(1_000_000)], "v": [float(i % 7) for i in range(1_000_000)]})
out = df.group_by("k").agg(pl.col("v").mean().alias("m"), pl.len().alias("n")).sort("k")
print(f"groups={out.height} first_mean={out['m'][0]:.3f}")
