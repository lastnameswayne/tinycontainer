#!/usr/bin/env python3
"""Picks the apps for E5's shared store. tinycontainer has one namespace, so two images that
put different content at the same path overwrite each other. Finds the largest group of
apps (that passed the smoke test) whose files don't conflict: greedy from every possible
starting app, keeping the biggest group (ties go to the higher-priority start). Conflicts
in __pycache__ and under /app/ are ignored: /app only holds each image's own build inputs
(requirements.txt, Dockerfile, app.py) which no app reads - sway runs the uploaded script
- and stale .pyc files are only recompiled.
Writes results/union_apps.txt and results/raw/union_conflicts.csv.
"""
import csv
import os
from pathlib import Path

REPO = Path(os.environ.get("REPO", Path(__file__).resolve().parents[1]))
RESULTS = Path(os.environ.get("RESULTS", REPO / "results"))
MAX_APPS = int(os.environ.get("E5_MAX_APPS", 60))
# the apps a typical workload would lean on first
PRIORITY = ["testappscipy", "testapp3", "ml-inference", "testapp2", "testappplotting",
            "numpy", "scipy", "pandas", "hello", "torch", "reader"]


def ignored(path):
    return "/__pycache__/" in path or path.startswith("/app/")


# reader walks the whole filesystem: in a shared store it would load every image's files
EXCLUDE = set(os.environ.get("E5_EXCLUDE", "reader").split())
ok = [a for a in (RESULTS / "apps_ok.txt").read_text().split() if a not in EXCLUDE]
order = [a for a in PRIORITY if a in ok] + sorted(a for a in ok if a not in PRIORITY)
files_of = {}
for app in order:
    manifest = RESULTS / "raw" / "manifests" / f"app-{app}.csv"
    if manifest.exists():
        with open(manifest) as f:
            files_of[app] = {r["path"]: r["sha1"] for r in csv.DictReader(f) if not ignored(r["path"])}
order = [a for a in order if a in files_of]


def greedy(start):
    owner = {}  # path -> (sha1, app)
    kept, conflicts = [], []
    for app in [start] + [a for a in order if a != start]:
        files = files_of[app]
        clash = [(p, owner[p][1]) for p, h in files.items() if p in owner and owner[p][0] != h]
        if clash:
            conflicts.append({"app": app, "conflicting_paths": len(clash), "example": clash[0][0],
                              "with_app": clash[0][1]})
            continue
        kept.append(app)
        for p, h in files.items():
            owner.setdefault(p, (h, app))
        if len(kept) >= MAX_APPS:
            break
    return kept, conflicts


best = max((greedy(start) for start in order), key=lambda kc: len(kc[0]))
kept, conflicts = best

(RESULTS / "union_apps.txt").write_text("".join(a + "\n" for a in kept))
with open(RESULTS / "raw" / "union_conflicts.csv", "w", newline="") as f:
    w = csv.DictWriter(f, fieldnames=["app", "conflicting_paths", "example", "with_app"])
    w.writeheader()
    w.writerows(conflicts)
print(f"union: {len(kept)} apps; skipped {len(conflicts)} with conflicting files")
