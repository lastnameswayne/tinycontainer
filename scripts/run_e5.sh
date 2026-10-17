#!/usr/bin/env bash
# E5/E6: one store holding many images (how a shared worker sees them), then the workload.
# Only apps whose files don't conflict can share tinycontainer's single namespace; see
# select_union.py.
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"
"$PY" "$here/select_union.py"
APPS=$(tr '\n' ' ' < "$RESULTS/union_apps.txt")
export APPS
"$here/prepare_store.sh" union $APPS
"$PY" "$here/run_e5.py"
