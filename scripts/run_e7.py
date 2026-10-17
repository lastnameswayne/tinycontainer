#!/usr/bin/env python3
"""E7: hedged remote requests under injected network delay (run by run_e7.sh, which sets
up netem on the fileserver port and tears it down again).

1. Pilot: PILOT cold runs of KEY_APP without hedging; the hedge threshold is the p90 of
   per-fetch remote latency in the pilot. The threshold comes from the delayed network
   rather than from E6 (localhost), otherwise nearly every request would be hedged.
2. RUNS cold runs of KEY_APP with hedging off and RUNS with hedging on, interleaved.
Writes results/e7_hedging.csv (one row per remote fetch) and results/raw/e7_runs.csv.
"""
import math
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
import harness as h  # noqa: E402

PILOT = int(os.environ.get("E7_PILOT", 10))
RUNS = int(os.environ.get("E7_RUNS", 30))
APP = os.environ.get("KEY_APP", "testappscipy")

runs = h.CsvLog(h.RESULTS / "raw" / "e7_runs.csv",
                ["run_id", "phase", "hedge", "hedge_ms", "rep", "t_first_output_ms", "t_exit_ms", "exit_code",
                 "server_fetches", "fs_file_fetches", "fs_file_value_bytes", "fs_response_bytes", "error"])
fetches = h.CsvLog(h.RESULTS / "e7_hedging.csv",
                   ["run_id", "phase", "hedge", "hedge_ms", "latency_us", "size_bytes", "hedge_state", "path"])


def remote_fetches(run_id):
    with h.worker_db() as con:
        return con.execute("""SELECT latency_us, size_bytes, hedge, path FROM file_events
                              WHERE run_id = ? AND op = 'lookup' AND tier = 'remote'""", (run_id,)).fetchall()


def one(phase, hedge_ms, rep):
    set_cache_state("cold", SWAY_HEDGE_MS=str(hedge_ms))
    r = h.sway_run(APP)
    hedge = "on" if hedge_ms else "off"
    runs.write(dict(r, phase=phase, hedge=hedge, hedge_ms=hedge_ms, rep=rep))
    rows = remote_fetches(r["run_id"]) if r["run_id"] else []
    for lat, size, state, path in rows:
        fetches.write({"run_id": r["run_id"], "phase": phase, "hedge": hedge, "hedge_ms": hedge_ms,
                       "latency_us": lat, "size_bytes": size, "hedge_state": state, "path": path})
    print(f"{phase} {hedge} rep{rep}: exit={r['t_exit_ms']} fetches={len(rows)}", flush=True)
    return [lat for lat, *_ in rows]


pilot = []
for rep in range(PILOT):
    pilot += one("pilot", 0, rep)
pilot.sort()
p90_ms = max(1, math.ceil(pilot[int(0.9 * (len(pilot) - 1))] / 1000))
(h.RESULTS / "raw" / "e7_threshold.txt").write_text(f"hedge threshold = pilot p90 = {p90_ms} ms "
                                                     f"({len(pilot)} fetches over {PILOT} runs)\n")
print(f"hedge threshold {p90_ms} ms", flush=True)

for rep in range(RUNS):
    one("main", 0, rep)
    one("main", p90_ms, rep)
set_cache_state("cold")
