#!/usr/bin/env python3
"""E1: tinycontainer at each cache level vs docker pull + run, for one app (its store is
prepared by run_e1.sh).

Each repetition runs every condition once, interleaved, so drift over time hits all
conditions equally:
  cold (L3)          every file comes from the fileserver
  warm (L2)          files from the worker's disk cache (filled by this repetition's cold run)
  hot (L1)           files from memory: the next run on the same live worker
  docker_pull_run    the baseline: worker page cache dropped, isolated dockerd emptied,
                     then `docker pull` + `docker run`, timed as one sequence
  docker_run_cached  reference: `docker run` again with the image present
Whether each tinycontainer run was really served by the tier on its label is checked
afterwards from file_events (analyze.py -> e1_tier_check.csv).
Appends to results/e1_coldstart.csv (summary columns) and results/raw/e1_runs.csv (all).
"""
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
import harness as h  # noqa: E402

app = sys.argv[1]
REPS = int(os.environ.get("E1_REPS", 5))
LEVEL = {"cold": "L3", "warm": "L2", "hot": "L1"}

summary = h.CsvLog(h.RESULTS / "e1_coldstart.csv",
                   ["run_id", "app", "condition", "rep", "t_first_output_ms", "t_exit_ms", "exit_code"])
raw = h.CsvLog(h.RESULTS / "raw" / "e1_runs.csv",
               ["run_id", "app", "condition", "rep", "t_first_output_ms", "t_exit_ms", "exit_code",
                "pull_ms", "client_total_ms", "worker_first_output_ms", "worker_exit_ms", "memory_hits",
                "disk_hits", "server_fetches", "negative_lookups", "dropped_events", "fs_file_fetches",
                "fs_file_value_bytes", "fs_response_bytes", "fs_not_found", "fs_dir_fetches", "error"])

for rep in range(REPS):
    for cond in ("cold", "warm", "hot"):
        set_cache_state(cond)
        r = dict(h.sway_run(app), app=app, condition=cond, rep=rep)
        summary.write(r)
        raw.write(r)
        print(f"{app} rep{rep} {cond} ({LEVEL[cond]}): first={r['t_first_output_ms']} "
              f"exit={r['t_exit_ms']} rc={r['exit_code']}", flush=True)

    h.drop_page_cache()
    h.docker_reset()
    pr = dict(h.docker_pull_run(app), run_id=f"docker-{app}-{rep}-pullrun", app=app,
              condition="docker_pull_run", rep=rep)
    rc = dict(h.docker_run_cached(app), run_id=f"docker-{app}-{rep}-cached", app=app,
              condition="docker_run_cached", rep=rep)
    for r in (pr, rc):
        summary.write(r)
        raw.write(r)
    print(f"{app} rep{rep} docker: pull+run first={pr['t_first_output_ms']} (pull {pr['pull_ms']}) "
          f"cached first={rc['t_first_output_ms']}", flush=True)

with open(h.RESULTS / "e1_done.txt", "a") as f:
    f.write(app + "\n")
