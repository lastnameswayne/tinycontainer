import random
from rapidfuzz import fuzz, process

# first output marks "imports done"
print("ready: t-rapidfuzz")

rng = random.Random(0)
names = ["".join(rng.choices("abcdefghijklmnopqrstuvwxyz", k=10)) for _ in range(50_000)]
best = process.extract("hellothere", names, scorer=fuzz.WRatio, limit=3)
print(f"best={[(m, round(s, 1)) for m, s, _ in best]}")
