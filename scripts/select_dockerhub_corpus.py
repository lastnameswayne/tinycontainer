#!/usr/bin/env python3
"""E4 corpus, Docker Hub part. Selection rule (reproducible, recorded with the date):

  Every official Docker image (library/*) in order of Docker Hub pull count; for each,
  the `latest` tag (if it has a linux/amd64 image) plus up to EXTRA_TAGS more of its most
  recently updated linux/amd64 tags whose content differs from the ones already taken.
  Stop at MAX_IMAGES. Images are pulled from AWS's public mirror of the official images
  (public.ecr.aws/docker/library), which has no anonymous pull limit from inside AWS.

Writes scripts/corpus/dockerhub_candidates.csv. Only Docker Hub's listing API is used
here; nothing is pulled.
"""
import csv
import datetime
import json
import sys
import time
import urllib.request
from pathlib import Path

MAX_IMAGES = int(sys.argv[1]) if len(sys.argv) > 1 else 450
EXTRA_TAGS = int(sys.argv[2]) if len(sys.argv) > 2 else 4
out = Path(__file__).parent / "corpus" / "dockerhub_candidates.csv"


def get(url):
    for attempt in range(5):
        try:
            with urllib.request.urlopen(url, timeout=30) as r:
                return json.load(r)
        except Exception:
            time.sleep(2 + 3 * attempt)
    return {}


repos, url = [], "https://hub.docker.com/v2/repositories/library/?page_size=100"
while url:
    page = get(url)
    repos += [(r["name"], r.get("pull_count", 0)) for r in page.get("results", [])]
    url = page.get("next")
repos.sort(key=lambda r: -r[1])

rows = []
for name, pulls in repos:
    tags = get(f"https://hub.docker.com/v2/repositories/library/{name}/tags/?page_size=50&ordering=last_updated").get("results", [])
    amd64 = []
    for t in tags:
        img = next((i for i in t.get("images", []) if i.get("os") == "linux" and i.get("architecture") == "amd64"), None)
        if img and img.get("digest"):
            amd64.append((t["name"], img["digest"], img.get("size", 0)))
    picked, digests = [], set()
    for tag, digest, size in sorted(amd64, key=lambda x: x[0] != "latest"):
        if digest in digests:
            continue
        if tag != "latest" and len(picked) >= 1 + EXTRA_TAGS:
            break
        picked.append((tag, digest, size))
        digests.add(digest)
    for tag, digest, size in picked:
        rows.append({"repo": name, "tag": tag, "hub_pull_count": pulls, "compressed_bytes": size, "digest": digest})
        if len(rows) >= MAX_IMAGES:
            break
    print(f"{len(rows):4d} {name} ({pulls}) {[p[0] for p in picked]}", file=sys.stderr)
    if len(rows) >= MAX_IMAGES:
        break
    time.sleep(0.3)

with open(out, "w", newline="") as f:
    f.write(f"# selected {datetime.date.today()} by select_dockerhub_corpus.py\n")
    w = csv.DictWriter(f, fieldnames=list(rows[0]), lineterminator="\n")
    w.writeheader()
    w.writerows(rows)
print(f"wrote {len(rows)} candidates to {out}", file=sys.stderr)
