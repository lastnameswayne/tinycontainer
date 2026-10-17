#!/usr/bin/env bash
# Runs every experiment from scratch on the worker box (as root), after setup_host.sh on
# both boxes, then derives the datasets and plots. Expect most of a day; E4 alone builds
# ~45 images.
#   RESET=1 ./run_all.sh      also deletes previous results and the worker's runs.db
#   STEPS="run_e1.sh" ./run_all.sh   runs only the listed steps (then analysis + plots)
#   APPS="hello scipy" ./run_all.sh  overrides the apps chosen by the smoke test
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
user_apps=${APPS:-}   # an explicit APPS overrides the smoke test's list
source "$here/exp.env"

if [ "${RESET:-0}" = 1 ]; then
  "$here/worker_ctl.sh" stop
  rm -rf "$WORKER_DIR/runs.db" "$WORKER_DIR/filecache"
  find "$RESULTS" -maxdepth 1 \( -name '*.csv' -o -name '*_ok.txt' \) -delete
  rm -rf "$RESULTS/raw" "$RESULTS/plots"
fi
mkdir -p "$RESULTS/raw" "$RESULTS/plots"
"$here/record_machine.sh"

# Order matters: E0 must pass before anything is measured; E1 smoke-tests every app and
# writes results/apps_ok.txt, which every later step uses.
steps=(${STEPS:-run_e0.sh run_e1.sh run_e5.sh run_e8.sh run_e10.sh run_e7.sh run_e9.sh run_e4.sh})
for s in "${steps[@]}"; do
  # pick up results/apps_ok.txt written by the smoke test
  if [ -z "$user_apps" ]; then unset APPS; source "$here/exp.env"; fi
  log "=== $s (apps: $APPS)"
  if ! "$here/$s" 2>&1 | tee "$RESULTS/raw/${s%.sh}.log"; then
    log "$s failed; see results/raw/${s%.sh}.log"
    case $s in
      run_e0.sh) log "stopping: $s must pass before anything is measured"; exit 1 ;;
      *) log "continuing with the remaining steps" ;;
    esac
  fi
done

"$PY" "$here/analyze.py"
"$PY" "$here/plot_all.py"
"$here/worker_ctl.sh" stop
on_store fileserver_ctl.sh stop
log "done: results in $RESULTS"
