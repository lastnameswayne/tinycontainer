#!/usr/bin/env bash
# export_app.sh <store> <app>
# `sway export`s one app into the (running) fileserver store, pushes the image to the
# store box's registry for the docker baseline, writes the image's file manifest
# (results/raw/manifests/app-<app>.csv) and appends export statistics to
# results/raw/exports.csv: export time, files in the image, files that were new to the
# store, compressed bytes docker pulls, store size afterwards.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)   # absolute: this script cds into the app directory
source "$here/exp.env"
source "$here/lib/apps.sh"
store=$1 app=$2

# Compressed bytes `docker pull` downloads: the layer sizes of the linux/amd64 manifest.
# Newer docker pushes a multi-platform index, so follow it to the platform manifest.
registry_layer_bytes() {
  local repo=$1 accept m digest
  accept="application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json"
  accept="$accept, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
  m=$(curl -sf -H "Accept: $accept" "http://$REGISTRY/v2/$repo/manifests/latest") || return 1
  if echo "$m" | jq -e '.manifests' >/dev/null; then
    digest=$(echo "$m" | jq -r '[.manifests[] | select(.platform.os == "linux" and .platform.architecture == "amd64")][0].digest')
    m=$(curl -sf -H "Accept: $accept" "http://$REGISTRY/v2/$repo/manifests/$digest") || return 1
  fi
  echo "$m" | jq -e '[.layers[].size] | add'
}

exportlog=$RESULTS/raw/exports.csv
mkdir -p "$RESULTS/raw"
[ -f "$exportlog" ] || echo "store,app,export_s,image_files,new_files,registry_compressed_bytes,store_bytes_after" > "$exportlog"

log "exporting $app into store $store"
cd "$(app_dir "$app")"
t0=$(date +%s.%N)
out=$("$SWAY" export 2>&1 | tee "$EXP_ROOT/export-$store-$app.log")
t1=$(date +%s.%N)
files=$(echo "$out" | sed -n 's/.*Extracted image (\([0-9]*\) files).*/\1/p' | tail -1)
new=$(echo "$out" | sed -n 's/.*Synced with fileserver — \([0-9]*\) new files.*/\1/p' | tail -1)
if [ -z "$files" ] || [ "$files" = 0 ]; then
  log "export of $app extracted no files (see $EXP_ROOT/export-$store-$app.log)"; exit 1
fi

compressed=""
if [ "${NO_PUSH:-0}" != 1 ]; then  # E9 never pulls with docker, so it skips the push
  docker tag "sway-$app" "$REGISTRY/exp-$app"
  docker push -q "$REGISTRY/exp-$app" >/dev/null
  compressed=$(registry_layer_bytes "exp-$app") || compressed=""
fi
[ -f "$RESULTS/raw/manifests/app-$app.csv" ] || "$PY" "$here/dedup_corpus.py" hash "sway-$app" "app-$app" >/dev/null

echo "$store,$app,$(echo "$t1 - $t0" | bc),$files,$new,$compressed,$(on_store fileserver_ctl.sh du "$store")" >> "$exportlog"
