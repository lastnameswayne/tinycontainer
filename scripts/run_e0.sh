#!/usr/bin/env bash
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"
"$here/prepare_store.sh" "e1-$KEY_APP" "$KEY_APP"
"$PY" "$here/run_e0.py"
