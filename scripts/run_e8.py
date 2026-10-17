#!/usr/bin/env python3
"""E8: tinycontainer cold start vs docker pull + run as the round-trip time between the
worker and the store grows. run_e8.sh prepares the app's store, sets the added RTT and
calls this once per value.

  run_e8.py <added_rtt_ms> <app>
Appends to results/e8_rtt.csv.
"""
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
import harness as h  # noqa: E402

rtt = int(sys.argv[1])
apps = sys.argv[2:]
REPS = int(os.environ.get("E8_REPS", 5))

out = h.CsvLog(h.RESULTS / "e8_rtt.csv",
               ["run_id", "app", "condition", "added_rtt_ms", "rep", "t_first_output_ms", "t_exit_ms",
                "pull_ms", "exit_code", "fs_file_fetches", "fs_response_bytes", "error"])
for rep in range(REPS):
    for app in apps:
        set_cache_state("cold")
        r = dict(h.sway_run(app), app=app, condition="cold", added_rtt_ms=rtt, rep=rep)
        out.write(r)
        h.drop_page_cache()
        h.docker_reset()
        d = dict(h.docker_pull_run(app), run_id=f"docker-{app}-{rtt}-{rep}", app=app,
                 condition="docker_pull_run", added_rtt_ms=rtt, rep=rep)
        out.write(d)
        print(f"+{rtt}ms {app} rep{rep}: cold={r['t_first_output_ms']} pull+run={d['t_first_output_ms']}", flush=True)
