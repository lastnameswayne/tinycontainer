#!/bin/bash
set -e

trap 'kill $(jobs -p) 2>/dev/null; exit 1' INT TERM

SKIP_EXPORT=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-export) SKIP_EXPORT=true; shift ;;
        *) shift ;;
    esac
done

cd ~/tinycontainerruntime

TIMEOUT=600
export SWAY_USERNAME="integration-tester"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${YELLOW}[TEST]${NC} $1"; }
pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; exit 1; }

run_app() {
    local dir=$1
    local expected=${2:-""}

    log "Testing $dir..."

    cd ~/tinycontainerruntime/"$dir"

    if [ "$SKIP_EXPORT" = false ]; then
        log "  Running sway export..."
        if ! timeout $TIMEOUT sway export > /dev/null 2>&1; then
            fail "$dir: sway export failed"
        fi
    fi

    log "  Running sway run..."
    OUTPUT=$(timeout $TIMEOUT sway run app.py 2>&1) || true

    echo "  Output: ${OUTPUT:0:100}..."

    if echo "$OUTPUT" | grep -qi "error\|panic\|fatal"; then
        fail "$dir: Output contains errors"
    fi

    if [ -n "$expected" ]; then
        if echo "$OUTPUT" | grep -q "$expected"; then
            pass "$dir: Found expected output '$expected'"
        else
            fail "$dir: Missing expected output '$expected'"
        fi
    else
        pass "$dir: Completed without errors"
    fi

    cd ~/tinycontainerruntime
}

# Run tests
log "Starting integration tests..."

run_app "testapp3" "hello world"
# run_app "testappplotting"  # Uncomment when ready

echo ""
echo -e "${GREEN}All integration tests passed!${NC}"
