import duckdb

# first output marks "imports done"
print("ready: d-duckdb")

con = duckdb.connect()
r = con.execute("""
    SELECT i % 100 AS k, avg(sqrt(i)) AS m, count(*) AS n
    FROM range(5000000) t(i) GROUP BY k ORDER BY k LIMIT 3
""").fetchall()
print(f"rows={r}")
