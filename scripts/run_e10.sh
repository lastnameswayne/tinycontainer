#!/usr/bin/env bash
# E10: .pyc on vs off, on a subset of apps (E10_APPS, default: E8's subset), each in its own store.
set -euo pipefail
here=$(dirname "$0"); source "$here/exp.env"
E10_PREFERRED=${E10_PREFERRED:-"hello s-sqlite testappscipy w-fastapi w-django n-sympy d-polars d-duckdb dl-torch-mlp dl-tensorflow reader"}
ok=" $(tr '\n' ' ' < "$RESULTS/apps_ok.txt") "
E10_APPS=${E10_APPS:-$(for a in $E10_PREFERRED; do [[ $ok == *" $a "* ]] && echo -n "$a "; done)}
for app in $E10_APPS; do
  "$here/prepare_store.sh" "e10-$app" "$app"
  "$PY" "$here/run_e10.py" "$app"
done
