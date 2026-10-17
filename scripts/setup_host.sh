#!/usr/bin/env bash
# setup_host.sh worker|store|both   (Ubuntu 24.04, run as root)
#   store:  fileserver binary + TLS cert, registry:2 on $REGISTRY_PORT
#   worker: runc/fuse, worker + sway binaries, docker (trusting the store's registry),
#           an isolated dockerd for the baseline, python venv for analysis
#   both:   single-machine setup
set -euo pipefail
source "$(dirname "$0")/exp.env"
role=${1:?usage: setup_host.sh worker|store|both}

apt_install() { DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@" >/dev/null; }

install_base() {
  log "installing packages"
  apt-get update -qq
  apt_install ca-certificates curl jq git openssl iproute2 bc sqlite3 python3-venv
  if ! command -v docker >/dev/null; then
    log "installing docker"
    curl -fsSL https://get.docker.com | sh >/dev/null
  fi
  if ! go version 2>/dev/null | grep -qE 'go1\.(2[4-9]|[3-9][0-9])'; then
    log "installing go"
    curl -fsSL https://go.dev/dl/go1.24.11.linux-amd64.tar.gz | tar -C /usr/local -xz
    ln -sf /usr/local/go/bin/go /usr/local/bin/go
  fi
  mkdir -p "$BIN" "$RESULTS/plots" "$RESULTS/raw"
}

setup_store() {
  mkdir -p "$FS_DIR" "$STORES"
  (cd "$REPO/fileserver" && go build -o "$BIN/fileserver" .)
  if [ ! -f "$FS_DIR/server.crt" ]; then
    log "creating self-signed cert for the experiment fileserver"
    openssl req -x509 -newkey rsa:2048 -nodes -days 365 -subj "/CN=tc-exp-store" \
      -keyout "$FS_DIR/server.key" -out "$FS_DIR/server.crt" 2>/dev/null
  fi
  log "registry on :$REGISTRY_PORT"
  docker inspect tc-exp-registry >/dev/null 2>&1 || \
    docker run -d --restart unless-stopped --name tc-exp-registry -p "$REGISTRY_PORT:5000" \
      -v "$EXP_ROOT/registry:/var/lib/registry" registry:2 >/dev/null
}

setup_worker() {
  apt_install fuse3
  # docker's containerd.io already ships runc; Ubuntu's runc package conflicts with it
  command -v runc >/dev/null || apt_install runc
  mkdir -p "$WORKER_DIR" "$MOUNT"
  (cd "$REPO/filesystem" && go build -o "$BIN/worker" .)
  (cd "$REPO/sway" && go build -o "$BIN/sway" .)

  # Classic image store: with the containerd store (docker 29's default) `docker save`
  # writes gzip-compressed layers, which `sway export` can't read (it then uploads 0 files).
  # Also trust the store box's plain-HTTP registry.
  log "configuring docker: classic image store, trust registry $REGISTRY"
  python3 - "$REGISTRY" "$STORE_HOST" <<'PY'
import json, os, sys
p = "/etc/docker/daemon.json"
cfg = json.load(open(p)) if os.path.exists(p) else {}
cfg.setdefault("features", {})["containerd-snapshotter"] = False
if sys.argv[2] != "localhost":
    cfg["insecure-registries"] = sorted(set(cfg.get("insecure-registries", [])) | {sys.argv[1]})
json.dump(cfg, open(p, "w"), indent=2)
PY
  systemctl restart docker

  log "isolated dockerd ($DIND) for the docker baseline"
  local hostflag=()
  [ "$STORE_HOST" = localhost ] && hostflag=(--add-host host.docker.internal:host-gateway)
  docker inspect "$DIND" >/dev/null 2>&1 || \
    docker run -d --privileged --restart unless-stopped --name "$DIND" "${hostflag[@]}" \
      -v tc-exp-dind-data:/var/lib/docker -e DOCKER_TLS_CERTDIR= docker:27-dind \
      --host=unix:///var/run/docker.sock --insecure-registry="$DIND_REGISTRY" >/dev/null
  for _ in $(seq 60); do docker exec "$DIND" docker info >/dev/null 2>&1 && break; sleep 1; done

  log "python venv for the harness and analysis"
  [ -x "$PY" ] || python3 -m venv "$EXP_ROOT/venv"
  "$PY" -m pip install -q --upgrade pip
  "$PY" -m pip install -q numpy pandas matplotlib
}

install_base
case "$role" in
  store) setup_store ;;
  worker) setup_worker ;;
  both) setup_store; setup_worker ;;
  *) echo "usage: $0 worker|store|both" >&2; exit 2 ;;
esac
[ "$role" != store ] && "$(dirname "$0")/record_machine.sh"
log "setup ($role) done"
