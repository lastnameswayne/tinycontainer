import io
import numpy as np
import pyarrow as pa
import pyarrow.parquet as pq
import pyarrow.compute as pc

# first output marks "imports done"
print("ready: d-pyarrow")

rng = np.random.default_rng(0)
table = pa.table({"k": rng.integers(0, 100, 500_000), "v": rng.standard_normal(500_000)})
buf = io.BytesIO()
pq.write_table(table, buf, compression="zstd")
back = pq.read_table(io.BytesIO(buf.getvalue()))
print(f"parquet bytes={len(buf.getvalue())} rows={back.num_rows} mean={pc.mean(back['v']).as_py():.4f}")
