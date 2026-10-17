#!/usr/bin/env bash
# E8: round-trip-time sweep. Every value is added on top of the real worker <-> store RTT
# (recorded in results/machine.txt). Uses a subset of apps spanning image size and import
# shape (E8_APPS), each in its own store.
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"; source "$here/lib/netem.sh"
E8_RTTS=${E8_RTTS:-"0 2 10 40"}
E8_PREFERRED=${E8_PREFERRED:-"hello s-sqlite testappscipy w-fastapi w-django n-sympy d-polars d-duckdb dl-torch-mlp dl-tensorflow reader"}
ok=" $(tr '\n' ' ' < "$RESULTS/apps_ok.txt") "
E8_APPS=${E8_APPS:-$(for a in $E8_PREFERRED; do [[ $ok == *" $a "* ]] && echo -n "$a "; done)}

trap netem_off EXIT
for app in $E8_APPS; do
  netem_off
  "$here/prepare_store.sh" "e8-$app" "$app"
  for rtt in $E8_RTTS; do
    log "E8: $app +${rtt} ms RTT"
    netem_rtt "$rtt"
    echo "$app +${rtt}ms: $(tc qdisc show dev "$(netem_dev)" | tr '\n' ' ')" >> "$RESULTS/raw/e8_netem.txt"
    [ "$STORE_HOST" != localhost ] && ping -c 10 -i 0.2 -q "$STORE_HOST" | tail -1 >> "$RESULTS/raw/e8_netem.txt"
    "$PY" "$here/run_e8.py" "$rtt" "$app"
  done
done
