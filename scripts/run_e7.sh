#!/usr/bin/env bash
# E7: delay + jitter + a little loss on worker -> fileserver traffic only, so the tail is
# visible even inside one AZ. Uses KEY_APP in its own store.
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"; source "$here/lib/netem.sh"
NETEM=${NETEM:-"delay 5ms 3ms distribution normal loss 0.2%"}

netem_off
"$here/prepare_store.sh" "e7-$KEY_APP" "$KEY_APP"
trap netem_off EXIT
netem_on "$NETEM" "$FS_PORT"
{
  echo "netem on $(netem_dev), fileserver port $FS_PORT only: $NETEM"
  [ "$(netem_dev)" = lo ] && echo "(one box: applies in both directions)" || echo "(worker -> store direction only)"
  tc qdisc show dev "$(netem_dev)"
} > "$RESULTS/raw/e7_netem.txt"

"$PY" "$here/run_e7.py"
