#!/usr/bin/env bash
# prepare_store.sh <store> <app>...
# Restarts the experiment fileserver on a fresh, empty store and exports each app into it
# (export_app.sh), then leaves the worker in the cold state.
#
# The fileserver is left running on purpose: its index is rebuilt from blobs on restart,
# and blobs are named by content hash only, so two paths with identical content survive a
# restart as one path. Never restart it between export and runs.
set -euo pipefail
here=$(dirname "$0")
source "$here/exp.env"
store=$1; shift

"$here/worker_ctl.sh" stop
on_store fileserver_ctl.sh fresh "$store"
for app in "$@"; do
  "$here/export_app.sh" "$store" "$app"
done
"$here/set_cache_state.sh" cold
