"""Shared helpers for the experiment scripts: timed sway / docker runs and CSV output.

All timings use time.monotonic(). For sway runs:

  t_exit_ms         client-side, from `sway run` invocation to container exit
  t_first_output_ms client-side, from `sway run` invocation to the first stdout byte

`sway run` does not stream output (the worker replies after the container exits), so
the worker timestamps the first stdout byte and the exit relative to when the /run
request reached it, and reports how long the handler took in total (handler_ms).
Everything before the handler (CLI start, script upload, request) is
client_total - handler_ms, which converts the worker offsets to the client's clock.
Post-exit bookkeeping (runc delete, event flush, DB write) is therefore excluded.
"""

import csv
import json
import os
import re
import sqlite3
import ssl
import subprocess
import time
import urllib.request
from pathlib import Path

REPO = Path(os.environ.get("REPO", Path(__file__).resolve().parents[2]))
RESULTS = Path(os.environ.get("RESULTS", REPO / "results"))
WORKER_DIR = Path(os.environ.get("WORKER_DIR", "/opt/tc-exp/worker"))
FILESERVER_URL = os.environ.get("FILESERVER_URL", "https://localhost:9443")
SWAY = os.environ.get("SWAY", "sway")
REGISTRY = os.environ.get("REGISTRY", "localhost:5000")
DIND = os.environ.get("DIND", "tc-exp-dind")
DIND_REGISTRY = os.environ.get("DIND_REGISTRY", "host.docker.internal:5000")
MACHINE = os.environ.get("MACHINE", os.uname().nodename)

_insecure = ssl.create_default_context()
_insecure.check_hostname = False
_insecure.verify_mode = ssl.CERT_NONE


def fileserver_metrics(reset=False):
    url = FILESERVER_URL + ("/metrics/reset" if reset else "/metrics")
    req = urllib.request.Request(url, method="POST" if reset else "GET")
    with urllib.request.urlopen(req, context=_insecure, timeout=30) as r:
        return json.load(r)


def worker_db():
    return sqlite3.connect(WORKER_DIR / "runs.db", timeout=30)


def run_row(run_id):
    with worker_db() as con:
        con.row_factory = sqlite3.Row
        row = con.execute("SELECT * FROM runs WHERE id = ?", (run_id,)).fetchone()
        return dict(row) if row else None


_RUN_ID = re.compile(r"/run/(\d+)")


def app_dir(app):
    """Apps live in testapps/<app> or, for the benchmark suite, testapps/suite/<app>."""
    d = REPO / "testapps" / app
    return d if d.is_dir() else REPO / "testapps" / "suite" / app


def sway_run(app, username=None, timeout=1800):
    """Runs `sway run app.py` in the app's directory. Returns one result dict."""
    appdir = app_dir(app)
    script = "app.py" if (appdir / "app.py").exists() else "script.py"
    # usernames must not contain "_" (isScript() expects exactly one, before "app.py")
    env = dict(os.environ, SWAY_USERNAME=username or "exp" + app.replace("_", "").replace("-", ""))
    fileserver_metrics(reset=True)
    t0 = time.monotonic()
    p = subprocess.run([SWAY, "run", script], cwd=appdir, env=env,
                       capture_output=True, text=True, timeout=timeout)
    client_total_ms = (time.monotonic() - t0) * 1000
    fs = fileserver_metrics()

    m = _RUN_ID.findall(p.stdout)
    run_id = int(m[-1]) if m else None
    row = run_row(run_id) if run_id else None
    out = {
        "run_id": run_id,
        "client_total_ms": round(client_total_ms, 1),
        "exit_code": row["exit_code"] if row else p.returncode,
        "fs_file_fetches": fs["file_fetches"],
        "fs_file_value_bytes": fs["file_value_bytes"],
        "fs_response_bytes": fs["response_bytes"],
        "fs_not_found": fs["not_found"],
        "fs_dir_fetches": fs["dir_fetches"],
    }
    if row and row.get("handler_ms") is not None:
        pre_ms = client_total_ms - row["handler_ms"]
        out.update({
            "t_first_output_ms": round(pre_ms + row["first_output_ms"], 1),
            "t_exit_ms": round(pre_ms + row["exit_ms"], 1),
            "worker_first_output_ms": row["first_output_ms"],
            "worker_exit_ms": row["exit_ms"],
            "memory_hits": row["memory_cache_hits"],
            "disk_hits": row["disk_cache_hits"],
            "server_fetches": row["server_fetches"],
            "negative_lookups": row["negative_lookups"],
            "dropped_events": row["dropped_events"],
        })
    else:
        out.update({"t_first_output_ms": None, "t_exit_ms": None,
                    "error": (p.stdout + p.stderr)[-2000:]})
    return out


def _dind(*args, **kw):
    return subprocess.run(["docker", "exec", DIND, "docker", *args], **kw)


def drop_page_cache():
    subprocess.run(["sync"], check=True)
    Path("/proc/sys/vm/drop_caches").write_text("3\n")


def docker_reset():
    """Empties the isolated dockerd so the next pull fetches every layer."""
    _dind("system", "prune", "-af", capture_output=True, check=True)


def _docker_run_timed(ref, t0):
    """Starts `docker run` and returns (time of first stdout byte, time of exit, returncode),
    both relative to t0, in ms. Same limits and environment as the runc config: 1 CPU,
    8 GiB, 4096 pids, no network, unbuffered Python output."""
    p = subprocess.Popen(["docker", "exec", DIND, "docker", "run", "--rm", "-e", "PYTHONUNBUFFERED=1",
                          "--memory", "8g", "--memory-swap", "8g", "--pids-limit", "4096",
                          "--cpus", "1", "--network", "none", ref],
                         stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    first = p.stdout.read(1)
    t_first = time.monotonic()
    p.stdout.read()
    p.wait()
    t_exit = time.monotonic()
    return (round((t_first - t0) * 1000, 1) if first else None,
            round((t_exit - t0) * 1000, 1), p.returncode)


def docker_pull_run(app):
    """Image not on the machine: `docker pull` then `docker run`, timed as one sequence from
    the start of the pull. The caller empties dind and drops the page cache first."""
    ref = f"{DIND_REGISTRY}/exp-{app}"
    t0 = time.monotonic()
    pull = _dind("pull", "-q", ref, capture_output=True, text=True)
    pull_ms = round((time.monotonic() - t0) * 1000, 1)
    if pull.returncode != 0:
        return {"t_first_output_ms": None, "t_exit_ms": None, "pull_ms": pull_ms,
                "exit_code": pull.returncode, "error": pull.stderr[-2000:]}
    first, exit_, rc = _docker_run_timed(ref, t0)
    return {"t_first_output_ms": first, "t_exit_ms": exit_, "pull_ms": pull_ms, "exit_code": rc}


def docker_run_cached(app):
    """Reference point: `docker run` with the image already on the machine."""
    ref = f"{DIND_REGISTRY}/exp-{app}"
    first, exit_, rc = _docker_run_timed(ref, time.monotonic())
    return {"t_first_output_ms": first, "t_exit_ms": exit_, "exit_code": rc}


class CsvLog:
    """Appends rows to a CSV, writing the header once. Rows may carry extra keys;
    only `fields` are written."""

    def __init__(self, path, fields):
        self.path = Path(path)
        self.fields = fields
        self.path.parent.mkdir(parents=True, exist_ok=True)
        new = not self.path.exists() or self.path.stat().st_size == 0
        self.f = open(self.path, "a", newline="")
        self.w = csv.DictWriter(self.f, fieldnames=fields, extrasaction="ignore")
        if new:
            self.w.writeheader()

    def write(self, row):
        self.w.writerow(row)
        self.f.flush()
