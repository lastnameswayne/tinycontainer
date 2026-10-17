import re
import collections
import random

# first output marks "imports done"
print("ready: s-regex")

rng = random.Random(0)
words = ["alpha", "beta", "gamma", "delta", "error", "warning", "info", "debug"]
text = "\n".join(f"2024-01-{rng.randint(1, 28):02d} {rng.choice(words).upper()}: {' '.join(rng.choices(words, k=12))}"
                  for _ in range(100_000))
pattern = re.compile(r"^(\d{4}-\d{2}-\d{2}) (ERROR|WARNING): (.*)$", re.M)
counts = collections.Counter(m.group(2) for m in pattern.finditer(text))
print(f"matches={dict(sorted(counts.items()))}")
