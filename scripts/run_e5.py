#!/usr/bin/env python3
"""E5: cache tiers under a realistic workload (union store prepared by run_e5.sh).

For each disk-cache cap: start cold, then RUNS runs drawn from a Zipf-like app mix in a
seeded random order, on one live worker (memory cache persists across runs).
Appends to results/raw/e5_runs.csv; per-run tier shares are computed by analyze.py.
"""
import os
import random
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
import harness as h  # noqa: E402

RUNS = int(os.environ.get("E5_RUNS", 200))
CAPS = [int(c) for c in os.environ.get("E5_CAPS", f"{2 << 30} {512 << 20}").split()]
# Zipf-like mix over the apps that passed the smoke test (APPS); apps not listed get weight 5.
WEIGHTS = {"testappscipy": 30, "testapp3": 20, "ml-inference": 15, "testapp2": 10, "testappplotting": 10,
           "numpy": 10, "scipy": 10, "pandas": 8, "hello": 5, "torch": 4, "reader": 2}
MIX = {a: WEIGHTS.get(a, 5) for a in os.environ["APPS"].split()}
SEED = 5

apps = list(MIX)
sequence = random.Random(SEED).choices(apps, weights=[MIX[a] for a in apps], k=RUNS)

raw = h.CsvLog(h.RESULTS / "raw" / "e5_runs.csv",
               ["run_id", "seq", "app", "cache_cap_bytes", "t_first_output_ms", "t_exit_ms", "exit_code",
                "memory_hits", "disk_hits", "server_fetches", "negative_lookups", "dropped_events",
                "fs_file_fetches", "fs_file_value_bytes", "error"])
for cap in CAPS:
    set_cache_state("cold", SWAY_DISK_CACHE_BYTES=str(cap))
    for seq, app in enumerate(sequence):
        r = dict(h.sway_run(app), seq=seq, app=app, cache_cap_bytes=cap)
        raw.write(r)
        print(f"cap={cap} {seq} {app}: exit={r['t_exit_ms']} fetches={r['fs_file_fetches']}", flush=True)
set_cache_state("cold")
