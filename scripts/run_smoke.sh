#!/usr/bin/env bash
# Smoke test only (no measurements): run_smoke.sh [app ...], default every app.
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"; source "$here/lib/apps.sh"
[ $# -gt 0 ] || set -- $(all_apps)
"$PY" "$here/run_smoke.py" "$@" || true
