#!/usr/bin/env python3
"""Smoke test: export an app into a fresh store and run it once; classify any failure.

  run_smoke.py <app> [...]

Each app gets the store e1-<app>, which is left running so E1 can measure the app in the
same store without exporting it again. Results are appended to results/smoke.csv, and
results/apps_ok.txt is rewritten from it (the latest result per app). Later experiments
only use apps that passed. Exit status 0 if every app given passed, 1 otherwise.
"""
import csv
import os
import re
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
import harness as h  # noqa: E402

HERE = Path(__file__).parent

# first match wins
CATEGORIES = [
    ("timeout", r"timed out|TimeoutExpired"),
    ("out of memory", r"MemoryError|Killed|exit code 137|oom"),
    ("missing shared library", r"cannot open shared object|error while loading shared libraries"),
    ("import error", r"ModuleNotFoundError|ImportError"),
    ("read-only filesystem", r"Read-only file system|\[Errno 30\]|No usable temporary directory"),
    ("I/O error from the filesystem", r"Input/output error|\[Errno 5\]"),
    ("missing file", r"No such file or directory|FileNotFoundError|\[Errno 2\]"),
    ("too many processes/threads", r"Resource temporarily unavailable|can't start new thread|pthread_create"),
    ("worker or fileserver error", r"Failed to run container|invalid response from worker|connection refused"),
]


def classify(text):
    for name, pattern in CATEGORIES:
        if re.search(pattern, text, re.IGNORECASE):
            return name
    return "other"


def smoke(app):
    t0 = time.monotonic()
    exp = subprocess.run([str(HERE / "prepare_store.sh"), f"e1-{app}", app], capture_output=True, text=True)
    if exp.returncode != 0:
        detail = (exp.stdout + exp.stderr)[-800:]
        build = "Build failed" in detail
        return {"stage": "build" if build else "export", "ok": 0,
                "category": "build failed" if build else classify(detail), "exit_code": exp.returncode,
                "seconds": round(time.monotonic() - t0), "detail": detail}
    try:
        r = h.sway_run(app, timeout=1800)
        row = h.run_row(r["run_id"]) if r.get("run_id") else None
        text = (r.get("error") or "") + (row.get("stderr") or "" if row else "")
    except subprocess.TimeoutExpired:
        r, text = {"exit_code": None}, "timed out"
    ok = r.get("exit_code") == 0
    return {"stage": "run", "ok": int(ok), "category": "" if ok else classify(text),
            "exit_code": r.get("exit_code"), "seconds": round(time.monotonic() - t0),
            "files_fetched": r.get("fs_file_fetches"), "detail": "" if ok else text[-800:]}


log = h.CsvLog(h.RESULTS / "smoke.csv",
               ["app", "stage", "ok", "category", "exit_code", "seconds", "files_fetched", "detail"])
all_ok = True
for app in sys.argv[1:]:
    res = smoke(app)
    res["detail"] = res["detail"].replace("\n", " | ")
    log.write(dict(res, app=app))
    all_ok &= bool(res["ok"])
    print(f"{app}: {'ok' if res['ok'] else res['stage'] + ' FAILED (' + res['category'] + ')'}", flush=True)

latest = {}
with open(h.RESULTS / "smoke.csv") as f:
    for row in csv.DictReader(f):
        latest[row["app"]] = row["ok"] == "1"
(h.RESULTS / "apps_ok.txt").write_text("".join(a + "\n" for a, ok in latest.items() if ok))
sys.exit(0 if all_ok else 1)
