#!/usr/bin/env bash
# record_machine.sh            writes results/machine.txt: this box, the store box, and the
#                              network between them
# record_machine.sh --spec     prints this box's spec only (used over ssh for the store box)
set -euo pipefail
source "$(dirname "$0")/exp.env"

imds() { # EC2 instance metadata (IMDSv2)
  local token
  token=$(curl -s -m 2 -X PUT http://169.254.169.254/latest/api/token \
    -H "X-aws-ec2-metadata-token-ttl-seconds: 60" 2>/dev/null) || { echo n/a; return; }
  curl -s -m 2 -H "X-aws-ec2-metadata-token: $token" "http://169.254.169.254/latest/meta-data/$1" 2>/dev/null || echo n/a
}

spec() {
  echo "hostname: $(hostname)"
  echo "instance_type: $(imds instance-type)"
  echo "az: $(imds placement/availability-zone)"
  echo "kernel: $(uname -r)"
  echo "os: $(. /etc/os-release && echo "$PRETTY_NAME")"
  echo "cpu: $(lscpu | awk -F: '/Model name/ {gsub(/^ +/, "", $2); print $2; exit}')"
  echo "vcpus: $(nproc)"
  echo "ram: $(free -h | awk '/Mem:/ {print $2}')"
  echo "disks: $(lsblk -dno NAME,ROTA,SIZE,MODEL | tr '\n' ';')"
  echo "root_fs: $(df -h / | tail -1)"
  echo "docker: $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo n/a)"
  echo "runc: $(runc --version 2>/dev/null | head -1 || echo n/a)"
}

if [ "${1:-}" = --spec ]; then spec; exit 0; fi

mkdir -p "$RESULTS"
{
  echo "date: $(date -Is)"
  echo "## worker box"
  spec
  echo "## store box ($STORE_HOST)"
  if [ -n "$STORE_SSH" ]; then on_store record_machine.sh --spec; else echo "same machine"; fi
  echo "## network worker -> store"
  echo "fileserver: $FILESERVER_URL   registry: $REGISTRY"
  if [ "$STORE_HOST" != localhost ]; then
    ping -c 50 -i 0.2 -q "$STORE_HOST" | tail -2
  else
    echo "localhost"
  fi
} > "$RESULTS/machine.txt"
cat "$RESULTS/machine.txt"
