#!/usr/bin/env python3
"""E0: instrumentation checks, on KEY_APP (store prepared by run_e0.sh).

1. Sanity: for one cold run, the bytes the worker logged as fetched remotely must equal
   the file bytes the fileserver reports having served. Writes results/e0_sanity.csv.
2. Logging overhead: REPS cold runs with event logging on and REPS with it off,
   interleaved. Writes results/e0_logging_overhead.csv.
"""
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
from harness import RESULTS, CsvLog, sway_run, worker_db  # noqa: E402

REPS = int(os.environ.get("REPS", 10))
APP = os.environ.get("KEY_APP", "testappscipy")

set_cache_state("cold")
r = sway_run(APP)
with worker_db() as con:
    logged, n_remote, n_events, n_read = con.execute("""
        SELECT COALESCE(SUM(CASE WHEN tier='remote' AND op='lookup' THEN size_bytes END), 0),
               SUM(tier='remote' AND op='lookup'), COUNT(*), SUM(op='read')
        FROM file_events WHERE run_id = ?""", (r["run_id"],)).fetchone()
sanity = {
    "run_id": r["run_id"], "worker_remote_bytes": logged, "fileserver_file_value_bytes": r["fs_file_value_bytes"],
    "worker_remote_fetches": n_remote, "fileserver_file_fetches": r["fs_file_fetches"],
    "events": n_events, "read_events": n_read, "dropped_events": r.get("dropped_events"),
    "match": int(logged == r["fs_file_value_bytes"] and n_remote == r["fs_file_fetches"]),
}
CsvLog(RESULTS / "e0_sanity.csv", list(sanity)).write(sanity)
print("sanity:", sanity)
if not sanity["match"]:
    sys.exit("E0 sanity check failed: worker and fileserver disagree; fix instrumentation first")

log = CsvLog(RESULTS / "e0_logging_overhead.csv",
             ["run_id", "rep", "logging", "t_first_output_ms", "t_exit_ms", "exit_code"])
for rep in range(REPS):
    for logging in ("on", "off"):
        set_cache_state("cold", SWAY_EVENTS="1" if logging == "on" else "0")
        r = sway_run(APP)
        log.write(dict(r, rep=rep, logging=logging))
        print(rep, logging, r["t_exit_ms"])
set_cache_state("cold")
