#!/usr/bin/env bash
# One-off: wait for the current run_all.sh to finish, move the first (single-app) E5 data
# aside, then re-run E5 and the analysis/plots.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
source "$here/exp.env"
while pgrep -f "[s]cripts/run_all.sh" >/dev/null; do sleep 60; done
old=$RESULTS/raw/superseded_e5_single_app
mkdir -p "$old"
for f in raw/e5_runs.csv union_apps.txt raw/union_conflicts.csv raw/run_e5.log e5_tiers.csv e5_lookup_tiers.csv e6_read_latency.csv e6_negative.csv; do
  [ -e "$RESULTS/$f" ] && mv "$RESULTS/$f" "$old/"
done
STEPS="run_e5.sh" "$here/run_all.sh"
