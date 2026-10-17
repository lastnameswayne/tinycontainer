#!/usr/bin/env bash
# E4: build the dedup corpus, hash every image once, then replay it into an empty store
# under the four schemes. Corpus = every app image exported in E1 (it only has to build,
# not run on tinycontainer) + corpus/variants.csv + the first 30 repos in
# corpus/github_candidates.csv that build in under 10 minutes.
# Images built here are tagged exp-corpus/<id> and removed after hashing.
set -uo pipefail
here=$(dirname "$0")
source "$here/exp.env"
source "$here/lib/apps.sh"
corpus=$RESULTS/e4_corpus.csv
GITHUB_TARGET=${GITHUB_TARGET:-30}
MIN_FREE_GB=${MIN_FREE_GB:-15}
export EXP_TMP=$EXP_ROOT/tmp
mkdir -p "$EXP_TMP"
[ -f "$corpus" ] || echo "image_id,source,base_image,build_ok,build_s,note" > "$corpus"

done_already() { grep -q "^$1," "$corpus"; }
free_gb() { df -BG --output=avail "$EXP_ROOT" | tail -1 | tr -dc 0-9; }
record() { echo "$1,$2,$3,$4,$5,$6" >> "$corpus"; }

hash_image() { # ref id
  "$PY" "$here/dedup_corpus.py" hash "$1" "$2"
}

build_and_hash() { # id source base context_dir
  local id=$1 src=$2 base=$3 ctx=$4 t0 rc
  if [ "$(free_gb)" -lt "$MIN_FREE_GB" ]; then
    log "less than ${MIN_FREE_GB}G free, skipping $id"; record "$id" "$src" "$base" 0 0 "skipped: low disk"; return 1
  fi
  t0=$SECONDS
  timeout 600 docker build -q --platform linux/amd64 -t "exp-corpus/$id" "$ctx" > "$EXP_TMP/build-$id.log" 2>&1
  rc=$?
  local secs=$((SECONDS - t0))
  if [ $rc -ne 0 ]; then
    local why="build failed"; [ $rc -eq 124 ] && why="build timeout (>600s)"
    record "$id" "$src" "$base" 0 "$secs" "$why"; log "$id: $why"; return 1
  fi
  if hash_image "exp-corpus/$id" "$id"; then
    record "$id" "$src" "$base" 1 "$secs" ""
  else
    record "$id" "$src" "$base" 0 "$secs" "hash failed"
  fi
  docker rmi -f "exp-corpus/$id" >/dev/null 2>&1
  docker builder prune -af >/dev/null 2>&1
}

log "E4: app images from E1"
for hashes in "$RESULTS"/raw/hashes/app-*.bin; do
  [ -e "$hashes" ] || continue
  id=$(basename "$hashes" .bin)
  done_already "$id" && continue
  base=$(sed -n 's/^FROM //p' "$(app_dir "${id#app-}")/Dockerfile" | head -1)
  record "$id" app "$base" 1 0 ""
done

log "E4: variants"
tail -n +2 "$here/corpus/variants.csv" | while IFS=, read -r id base reqs; do
  done_already "$id" && continue
  ctx=$EXP_TMP/ctx-$id; rm -rf "$ctx"; mkdir -p "$ctx"
  cp -R "$REPO/testapps/testappscipy/." "$ctx/"
  sed -i -e "s|^FROM .*|FROM $base|" -e "s|^RUN pip install.*|RUN pip install --no-cache-dir $reqs|" "$ctx/Dockerfile"
  build_and_hash "$id" variant "$base" "$ctx"
  rm -rf "$ctx"
done

log "E4: GitHub repos"
ok=$(awk -F, '$2=="github" && $4==1' "$corpus" | wc -l)
grep -v '^#' "$here/corpus/github_candidates.csv" | tr -d '\r' | tail -n +2 | while IFS=, read -r rank repo stars commit base; do
  [ "$ok" -ge "$GITHUB_TARGET" ] && break
  id="gh-$(echo "$repo" | tr '/' '-' | tr '[:upper:]' '[:lower:]')"
  if done_already "$id"; then
    grep -q "^$id,github,.*,1," "$corpus" && ok=$((ok + 1))
    continue
  fi
  ctx=$EXP_TMP/ctx-$id; rm -rf "$ctx"
  if ! git clone -q --filter=blob:none "https://github.com/$repo" "$ctx" 2>/dev/null || ! git -C "$ctx" checkout -q "$commit" 2>/dev/null; then
    record "$id" github "$base" 0 0 "clone failed"; rm -rf "$ctx"; continue
  fi
  build_and_hash "$id" github "$base" "$ctx" && ok=$((ok + 1))
  rm -rf "$ctx"
done

log "E4: replaying corpus into the store"
"$PY" "$here/dedup_corpus.py" analyze
