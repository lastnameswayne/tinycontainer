#!/usr/bin/env bash
# E1 (and the data for E2/E3): every app gets its own fresh store, so "the image" is exactly
# that app's image. Each app is smoke-tested first in that store; apps that fail are
# recorded in results/smoke.csv and skipped.
#   E1_APPS="a b" run_e1.sh    only these apps (default: every app)
set -uo pipefail
here=$(dirname "$0"); source "$here/exp.env"; source "$here/lib/apps.sh"
done_file=$RESULTS/e1_done.txt   # apps whose E1 runs are complete; skipped on a re-run
touch "$done_file"
for app in ${E1_APPS:-$(all_apps)}; do
  grep -qx "$app" "$done_file" && continue
  if "$PY" "$here/run_smoke.py" "$app"; then
    "$PY" "$here/run_e1.py" "$app"
  fi
done
