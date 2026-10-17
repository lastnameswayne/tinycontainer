#!/usr/bin/env python3
"""E4 corpus, GitHub part. Selection rule (reproducible, recorded with the date):

  Walk the most-starred Python repositories on GitHub (gh search repos --language=python
  --sort=stars) in order and keep every repo whose *root* Dockerfile contains
  "pip install", pinning the default-branch commit. Stop at MAX_CANDIDATES candidates.
  build_corpus.sh then builds them in this order and keeps the first 30 that build
  successfully with `docker build` in under 10 minutes.

Writes scripts/corpus/github_candidates.csv. Needs an authenticated `gh`.
"""
import base64
import csv
import datetime
import json
import subprocess
import sys
import time
from pathlib import Path

MAX_CANDIDATES = int(sys.argv[1]) if len(sys.argv) > 1 else 60
SEARCH_LIMIT = 1000
out = Path(__file__).parent / "corpus" / "github_candidates.csv"


def gh(*args):
    time.sleep(0.5)  # stay under GitHub's secondary (burst) rate limit
    p = subprocess.run(["gh", *args], capture_output=True, text=True)
    return p.stdout if p.returncode == 0 else None


def search_pages():
    for page in range(1, SEARCH_LIMIT // 100 + 1):
        body = gh("api", f"search/repositories?q=language:python&sort=stars&order=desc&per_page=100&page={page}")
        if body is None:
            time.sleep(60)
            body = gh("api", f"search/repositories?q=language:python&sort=stars&order=desc&per_page=100&page={page}")
        for item in json.loads(body)["items"]:
            yield {"fullName": item["full_name"], "stargazersCount": item["stargazers_count"],
                   "defaultBranch": item["default_branch"]}
        time.sleep(3)


rows = []
for rank, r in enumerate(search_pages(), 1):
    body = gh("api", f"repos/{r['fullName']}/contents/Dockerfile", "--jq", ".content")
    if not body:
        continue
    text = base64.b64decode(body).decode(errors="replace")
    if "pip install" not in text:
        continue
    sha = (gh("api", f"repos/{r['fullName']}/commits/{r['defaultBranch']}", "--jq", ".sha") or "").strip()
    base = next((l.split()[1] for l in text.splitlines() if l.strip().upper().startswith("FROM ") and len(l.split()) > 1), "")
    rows.append({"rank": rank, "repo": r["fullName"], "stars": r["stargazersCount"],
                 "commit": sha, "base_image": base})
    print(f"{len(rows):3d} #{rank} {r['fullName']} ({r['stargazersCount']}) FROM {base}", file=sys.stderr)
    if len(rows) >= MAX_CANDIDATES:
        break

with open(out, "w", newline="") as f:
    f.write(f"# selected {datetime.date.today()} by select_github_corpus.py\n")
    w = csv.DictWriter(f, fieldnames=list(rows[0]), lineterminator="\n")
    w.writeheader()
    w.writerows(rows)
print(f"wrote {len(rows)} candidates to {out}", file=sys.stderr)
