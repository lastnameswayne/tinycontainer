import sqlite3
import random

# first output marks "imports done"
print("ready: s-sqlite")

con = sqlite3.connect(":memory:")
con.execute("CREATE TABLE t (k INTEGER, v REAL)")
rng = random.Random(0)
con.executemany("INSERT INTO t VALUES (?, ?)", [(rng.randrange(100), rng.random()) for _ in range(200_000)])
rows = con.execute("SELECT k, AVG(v), COUNT(*) FROM t GROUP BY k ORDER BY k").fetchall()
print(f"groups={len(rows)} first={rows[0][0]} avg={rows[0][1]:.4f}")
