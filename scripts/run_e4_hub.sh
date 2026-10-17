#!/usr/bin/env bash
# E4, Docker Hub part: pull each image in corpus/dockerhub_candidates.csv from AWS's public
# mirror of the official images, hash it (dedup_corpus.py hash), record it in
# results/e4_corpus.csv, delete it. E4_HUB_JOBS images at a time. Then re-run the analysis.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
source "$here/exp.env"
export EXP_TMP=$EXP_ROOT/tmp
mkdir -p "$EXP_TMP"
corpus=$RESULTS/e4_corpus.csv
MIRROR=${MIRROR:-public.ecr.aws/docker/library}
JOBS=${E4_HUB_JOBS:-3}

one() { # repo tag
  local repo=$1 tag=$2 id ref t0 secs
  id="hub-$repo-$(echo "$tag" | tr -c 'a-zA-Z0-9.\n' '-')"
  grep -q "^$id," "$corpus" && return 0
  ref=$MIRROR/$repo:$tag
  t0=$SECONDS
  if ! timeout 900 docker pull -q --platform linux/amd64 "$ref" >/dev/null 2>&1; then
    flock "$corpus" -c "echo '$id,dockerhub,$repo:$tag,0,$((SECONDS - t0)),pull failed' >> '$corpus'"; return 0
  fi
  if "$PY" "$here/dedup_corpus.py" hash "$ref" "$id" >/dev/null 2>&1; then
    flock "$corpus" -c "echo '$id,dockerhub,$repo:$tag,1,$((SECONDS - t0)),' >> '$corpus'"
  else
    flock "$corpus" -c "echo '$id,dockerhub,$repo:$tag,0,$((SECONDS - t0)),hash failed' >> '$corpus'"
  fi
  docker rmi -f "$ref" >/dev/null 2>&1
}
export -f one
export corpus MIRROR here

grep -v '^#' "$here/corpus/dockerhub_candidates.csv" | tail -n +2 | cut -d, -f1,2 | tr ',' ' ' \
  | xargs -P "$JOBS" -L 1 bash -c 'one "$0" "$1"'
docker image prune -f >/dev/null 2>&1
log "E4 hub: $(grep -c ',dockerhub,.*,1,' "$corpus") hashed, $(grep -c ',dockerhub,.*,0,' "$corpus") failed"
"$PY" "$here/dedup_corpus.py" analyze
