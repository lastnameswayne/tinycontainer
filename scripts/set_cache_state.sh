#!/usr/bin/env bash
# set_cache_state.sh cold|warm|hot
#   cold: empty filecache/, restart the worker (empties the memory cache), drop the OS page cache
#   warm: keep filecache/ from the previous run of the same app, restart the worker, drop the page cache
#   hot:  nothing; the next run is a second consecutive run on a live worker
set -euo pipefail
here=$(dirname "$0")
source "$here/exp.env"

drop_page_cache() { sync; echo 3 > /proc/sys/vm/drop_caches; }

case "${1:-}" in
  cold)
    "$here/worker_ctl.sh" stop
    rm -rf "$WORKER_DIR/filecache"
    drop_page_cache
    "$here/worker_ctl.sh" start ;;
  warm)
    "$here/worker_ctl.sh" stop
    drop_page_cache
    "$here/worker_ctl.sh" start ;;
  hot) ;;
  *) echo "usage: $0 cold|warm|hot" >&2; exit 2 ;;
esac
