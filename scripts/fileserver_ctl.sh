#!/usr/bin/env bash
# fileserver_ctl.sh start <store> | fresh <store> | stop | du <store> | metrics | reset-metrics
# Runs the experiment fileserver on $FS_PORT with its blob store in $STORES/<store>.
# Runs on the store box (the worker box calls it through on_store).
set -euo pipefail
source "$(dirname "$0")/exp.env"
pidfile=$FS_DIR/fileserver.pid

stop() {
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    kill "$(cat "$pidfile")"
    for _ in $(seq 50); do kill -0 "$(cat "$pidfile")" 2>/dev/null || break; sleep 0.1; done
  fi
  rm -f "$pidfile"
}

case "${1:-}" in
  fresh|start)
    store=${2:?store name required}
    stop
    [ "$1" = fresh ] && rm -rf "${STORES:?}/$store"
    mkdir -p "$STORES/$store"
    cd "$FS_DIR"
    FILESERVER_DIR="$STORES/$store" FILESERVER_ADDR=":$FS_PORT" \
      nohup "$BIN/fileserver" >> "$FS_DIR/fileserver.log" 2>&1 &
    echo $! > "$pidfile"
    for _ in $(seq 600); do
      curl -sk "https://localhost:$FS_PORT/metrics" >/dev/null && { log "fileserver up (store $store)"; exit 0; }
      sleep 0.5
    done
    log "fileserver did not start"; exit 1 ;;
  stop) stop ;;
  du) du -sb "$STORES/${2:?store name required}" | cut -f1 ;;
  reset-metrics) curl -sk -X POST "https://localhost:$FS_PORT/metrics/reset" ;;
  metrics) curl -sk "https://localhost:$FS_PORT/metrics" ;;
  *) echo "usage: $0 start|fresh <store> | stop | du <store> | metrics | reset-metrics" >&2; exit 2 ;;
esac
