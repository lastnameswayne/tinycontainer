import csv
import statistics

# first output marks "imports done"
print("ready: s-csv")

with open("/usr/local/share/tcdata/s-csv-data.csv", newline="") as f:
    rows = list(csv.DictReader(f))
by_city = {}
for r in rows:
    by_city.setdefault(r["city"], []).append(float(r["temp"]))
print(f"rows={len(rows)} cities={len(by_city)} median={statistics.median(by_city['oslo']):.2f}")
