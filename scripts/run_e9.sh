#!/usr/bin/env bash
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"
"$PY" "$here/run_e9.py"
