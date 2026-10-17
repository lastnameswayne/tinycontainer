#!/usr/bin/env bash
# E6 is derived from data other experiments record (E2/E3 from the E1 cold runs, E6 from
# the E5 workload), so this only runs the analysis.
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"
"$PY" "$here/analyze.py"
