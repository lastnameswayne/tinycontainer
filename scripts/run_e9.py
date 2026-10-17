#!/usr/bin/env python3
"""E9 (counterpart of the Lambda paper's Fig. 8): when a worker meets an image for the
first time, what share of the files it needs are already cached locally because other
images it ran had identical content?

For each seeded random order of the apps that passed the smoke test (results/apps_ok.txt;
E9_MAX caps how many):
  fresh store, empty worker cache; then for each image in turn:
    sway export it into the shared store (as a user uploading a new function would),
    restart the worker keeping its disk cache (memory emptied, page cache dropped; a
    restart is needed because a live worker keeps serving the old content of paths it
    already loaded), run it once.
The cache is kept across arrivals, so every image is new but arrives after the others.
Per-run tier shares are computed by analyze.py (e9_hit_rates.csv).
"""
import os
import random
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent / "lib"))
from cache import set_cache_state  # noqa: E402
import harness as h  # noqa: E402

HERE = Path(__file__).parent
SEEDS = [int(s) for s in os.environ.get("E9_SEEDS", "1 2").split()]
# E9_APPS_FILE: which apps arrive. Default: the apps that can share one store (E5's set);
# arbitrary images overwrite each other's files in tinycontainer's single namespace.
variants = (h.RESULTS / os.environ.get("E9_APPS_FILE", "union_apps.txt")).read_text().split()
MAX = int(os.environ.get("E9_MAX", len(variants)))

out = h.CsvLog(h.RESULTS / "raw" / "e9_runs.csv",
               ["run_id", "seed", "seq", "image", "t_first_output_ms", "t_exit_ms", "exit_code",
                "memory_hits", "disk_hits", "server_fetches", "fs_file_fetches", "fs_file_value_bytes", "error"])


def on_store(*args):
    store_ssh = os.environ.get("STORE_SSH", "")
    script = str(h.REPO / "scripts" / args[0])
    cmd = (store_ssh.split() + ["sudo", script] if store_ssh else [script]) + list(args[1:])
    subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL)


for seed in SEEDS:
    order = variants[:]
    random.Random(seed).shuffle(order)
    order = order[:MAX]
    subprocess.run([str(HERE / "worker_ctl.sh"), "stop"], check=True)
    on_store("fileserver_ctl.sh", "fresh", "e9")
    set_cache_state("cold")
    for seq, image in enumerate(order):
        subprocess.run([str(HERE / "export_app.sh"), "e9", image], check=True,
                       env=dict(os.environ, NO_PUSH="1"), stdout=subprocess.DEVNULL)
        set_cache_state("warm")
        r = dict(h.sway_run(image), seed=seed, seq=seq, image=image)
        out.write(r)
        print(f"seed{seed} #{seq} {image}: rc={r['exit_code']} fetched={r['fs_file_fetches']} "
              f"disk_hits={r.get('disk_hits')}", flush=True)
set_cache_state("cold")
