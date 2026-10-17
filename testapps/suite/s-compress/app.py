import hashlib
import zlib
import random

# first output marks "imports done"
print("ready: s-compress")

data = random.Random(0).randbytes(8 << 20) + b"abc" * (4 << 20)
packed = zlib.compress(data, 6)
assert zlib.decompress(packed) == data
print(f"in={len(data)} out={len(packed)} sha256={hashlib.sha256(data).hexdigest()[:16]}")
