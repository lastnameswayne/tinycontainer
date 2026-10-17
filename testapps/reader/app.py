# Worst case for lazy loading: read (nearly) every file in the image, so the
# working set is close to the image size. Pseudo filesystems are skipped.
import os

print("ready: reader", flush=True)

SKIP = {"/proc", "/sys", "/dev"}
files = 0
nbytes = 0
errors = 0
for root, dirs, names in os.walk("/"):
    dirs[:] = [d for d in dirs if os.path.join(root, d) not in SKIP]
    for name in names:
        p = os.path.join(root, name)
        try:
            if os.path.islink(p) or not os.path.isfile(p):
                continue
            with open(p, "rb") as f:
                while chunk := f.read(1 << 20):
                    nbytes += len(chunk)
            files += 1
        except OSError:
            errors += 1
print(f"read files={files} bytes={nbytes} errors={errors}")
