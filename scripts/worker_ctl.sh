#!/usr/bin/env bash
# worker_ctl.sh start | stop | restart
# Runs the experiment worker on $WORKER_PORT with its FUSE mount at $MOUNT.
# SWAY_EVENTS, SWAY_HEDGE_MS, SWAY_DISK_CACHE_BYTES and SWAY_PYC are passed through.
set -euo pipefail
source "$(dirname "$0")/exp.env"
pidfile=$WORKER_DIR/worker.pid

stop() {
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    kill "$(cat "$pidfile")"
    for _ in $(seq 100); do kill -0 "$(cat "$pidfile")" 2>/dev/null || break; sleep 0.1; done
    kill -9 "$(cat "$pidfile")" 2>/dev/null || true
  fi
  rm -f "$pidfile"
  fusermount3 -u "$MOUNT" 2>/dev/null || umount -l "$MOUNT" 2>/dev/null || true
}

start() {
  mkdir -p "$WORKER_DIR" "$MOUNT"
  cd "$WORKER_DIR"
  WORKER_ADDR=":$WORKER_PORT" FILESERVER_URL="$FILESERVER_URL" \
  SWAY_EVENTS="${SWAY_EVENTS:-1}" SWAY_HEDGE_MS="${SWAY_HEDGE_MS:-0}" \
  SWAY_DISK_CACHE_BYTES="${SWAY_DISK_CACHE_BYTES:-0}" SWAY_PYC="${SWAY_PYC:-1}" \
    nohup "$BIN/worker" "$MOUNT" >> "$WORKER_DIR/worker.log" 2>&1 &
  echo $! > "$pidfile"
  for _ in $(seq 100); do
    if mountpoint -q "$MOUNT" && curl -s -o /dev/null "$WORKER_URL/"; then
      return 0
    fi
    sleep 0.1
  done
  log "worker did not start"; exit 1
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  *) echo "usage: $0 start|stop|restart" >&2; exit 2 ;;
esac
