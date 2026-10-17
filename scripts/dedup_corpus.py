#!/usr/bin/env python3
"""E4: file-level vs chunk-level deduplication across a corpus of images.

  dedup_corpus.py hash <image-ref> <image-id>
      docker save the image to a temp file, reconstruct its rootfs and hash every file
      once under all four schemes. Writes results/raw/hashes/<image-id>.bin and a
      per-file manifest results/raw/manifests/<image-id>.csv (path, size, sha1, link).

  dedup_corpus.py analyze
      Adds the images listed in results/e4_corpus.csv (build_ok=1) to an initially empty
      store one at a time, sorted by image_id and in two seeded random permutations,
      and writes results/e4_dedup.csv and results/e4_store_totals.csv.
"""

import csv
import os
import random
import struct
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
import imagewalk  # noqa: E402

REPO = Path(os.environ.get("REPO", Path(__file__).resolve().parents[1]))
RESULTS = Path(os.environ.get("RESULTS", REPO / "results"))
HASHES = RESULTS / "raw" / "hashes"
MANIFESTS = RESULTS / "raw" / "manifests"
TMP = os.environ.get("EXP_TMP", os.environ.get("EXP_ROOT", "/tmp"))
ALL_SCHEMES = ["file"] + list(imagewalk.SCHEMES)
PERMUTATION_SEEDS = [1, 2]


def write_hashes(path, entries):
    """Binary layout per scheme: name length, name, count, then count x (20-byte digest, u64 size)."""
    objs = {s: [] for s in ALL_SCHEMES}
    for rec in entries.values():
        if not isinstance(rec, imagewalk.FileRec):
            continue
        objs["file"].append((rec.sha1, rec.size))
        for s, chunks in rec.chunks.items():
            objs[s].extend(chunks)
    with open(path, "wb") as f:
        for s in ALL_SCHEMES:
            name = s.encode()
            f.write(struct.pack("<H", len(name)) + name + struct.pack("<Q", len(objs[s])))
            for digest, size in objs[s]:
                f.write(digest + struct.pack("<Q", size))


def read_hashes(path):
    out = {}
    with open(path, "rb") as f:
        while True:
            head = f.read(2)
            if not head:
                break
            (n,) = struct.unpack("<H", head)
            name = f.read(n).decode()
            (count,) = struct.unpack("<Q", f.read(8))
            data = f.read(28 * count)
            out[name] = [(data[i:i + 20], struct.unpack_from("<Q", data, i + 20)[0])
                         for i in range(0, len(data), 28)]
    return out


def write_manifest(path, entries):
    with open(path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["path", "size_bytes", "sha1", "link_target"])
        for p in sorted(entries):
            rec = entries[p]
            if isinstance(rec, imagewalk.FileRec):
                w.writerow([p, rec.size, rec.sha1.hex(), ""])
            else:
                resolved = imagewalk.resolve(entries, p)
                if resolved:
                    target = entries[resolved]
                    w.writerow([p, target.size, target.sha1.hex(), resolved])


def cmd_hash(ref, image_id):
    HASHES.mkdir(parents=True, exist_ok=True)
    MANIFESTS.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=TMP, suffix=".tar") as tmp:
        subprocess.run(["docker", "save", "-o", tmp.name, ref], check=True)
        entries = imagewalk.walk_image_tar(tmp.name)
    write_hashes(HASHES / f"{image_id}.bin", entries)
    write_manifest(MANIFESTS / f"{image_id}.csv", entries)
    files = [r for r in entries.values() if isinstance(r, imagewalk.FileRec)]
    print(f"{image_id}: {len(files)} files, {sum(r.size for r in files)} bytes")


def cmd_analyze():
    with open(RESULTS / "e4_corpus.csv") as f:
        ids = sorted(r["image_id"] for r in csv.DictReader(f) if r["build_ok"] == "1")
    ids = [i for i in ids if (HASHES / f"{i}.bin").exists()]
    orderings = {"sorted": ids}
    for seed in PERMUTATION_SEEDS:
        perm = ids[:]
        random.Random(seed).shuffle(perm)
        orderings[f"perm{seed}"] = perm

    rows, totals = [], []
    for ordering, order in orderings.items():
        store = {s: set() for s in ALL_SCHEMES}
        store_bytes = {s: 0 for s in ALL_SCHEMES}
        for pos, image_id in enumerate(order):
            hashes = read_hashes(HASHES / f"{image_id}.bin")
            image_bytes = sum(size for _, size in hashes["file"])
            for s in ALL_SCHEMES:
                unique = 0
                for digest, size in hashes[s]:
                    if digest not in store[s]:
                        store[s].add(digest)
                        unique += size
                store_bytes[s] += unique
                rows.append({
                    "ordering": ordering, "order": pos, "image_id": image_id,
                    "image_bytes": image_bytes, "scheme": s, "unique_bytes": unique,
                    "pct_unique": round(100 * unique / image_bytes, 4) if image_bytes else 0,
                    "n_objects": len(store[s]), "store_bytes": store_bytes[s],
                })
        for s in ALL_SCHEMES:
            totals.append({"ordering": ordering, "scheme": s, "images": len(order),
                           "store_bytes": store_bytes[s], "n_objects": len(store[s])})
        print(f"{ordering}: " + ", ".join(f"{s}={store_bytes[s] / 1e9:.2f}GB/{len(store[s])}obj"
                                           for s in ALL_SCHEMES))

    for name, data in [("e4_dedup.csv", rows), ("e4_store_totals.csv", totals)]:
        with open(RESULTS / name, "w", newline="") as f:
            w = csv.DictWriter(f, fieldnames=list(data[0]))
            w.writeheader()
            w.writerows(data)


if __name__ == "__main__":
    if sys.argv[1:2] == ["hash"] and len(sys.argv) == 4:
        cmd_hash(sys.argv[2], sys.argv[3])
    elif sys.argv[1:2] == ["analyze"]:
        cmd_analyze()
    else:
        sys.exit(__doc__)
