import os
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parents[1]


def set_cache_state(state, **env):
    """Calls set_cache_state.sh; extra env (e.g. SWAY_EVENTS='0') is passed to the worker."""
    subprocess.run([str(HERE / "set_cache_state.sh"), state], check=True,
                   env=dict(os.environ, **env), stdout=subprocess.DEVNULL)


def worker(action, **env):
    subprocess.run([str(HERE / "worker_ctl.sh"), action], check=True,
                   env=dict(os.environ, **env), stdout=subprocess.DEVNULL)
