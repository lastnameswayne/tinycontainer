#!/usr/bin/env python3
"""E10: what precompiled .pyc files are worth. Before the mtime fix, the worker hid
__pycache__ and reported mtime 0, so Python compiled every imported module from source on
every run. SWAY_PYC=0 restores that behaviour.

  run_e10.py <app>     (run_e10.sh prepares the app's store first)

Each repetition: cold and hot with .pyc files used, then cold and hot without, interleaved.
Appends to results/e10_pyc.csv.
"""
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
import harness as h  # noqa: E402

app = sys.argv[1]
REPS = int(os.environ.get("E10_REPS", 5))

out = h.CsvLog(h.RESULTS / "e10_pyc.csv",
               ["run_id", "app", "pyc", "state", "rep", "t_first_output_ms", "t_exit_ms", "exit_code",
                "fs_file_fetches", "fs_file_value_bytes", "negative_lookups", "error"])
for rep in range(REPS):
    for pyc in ("on", "off"):
        for state in ("cold", "hot"):
            set_cache_state(state, SWAY_PYC="1" if pyc == "on" else "0")
            r = dict(h.sway_run(app), app=app, pyc=pyc, state=state, rep=rep)
            out.write(r)
            print(f"{app} rep{rep} pyc={pyc} {state}: first={r['t_first_output_ms']} exit={r['t_exit_ms']}", flush=True)
set_cache_state("cold")
