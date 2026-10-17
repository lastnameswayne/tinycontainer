import csv, os, random

os.makedirs("/usr/local/share/tcdata", exist_ok=True)
rng = random.Random(0)
with open("/usr/local/share/tcdata/s-csv-data.csv", "w", newline="") as f:
    w = csv.writer(f)
    w.writerow(["city", "day", "temp"])
    for i in range(200_000):
        w.writerow([rng.choice(["oslo", "lima", "pune", "kyiv", "nuuk"]), i % 365, round(rng.gauss(10, 8), 2)])
