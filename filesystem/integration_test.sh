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

cd ~/tinycontainerruntime/testapps

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

    log "Testing $dir..."

    cd ~/tinycontainerruntime/testapps/"$dir"

    if [ "$SKIP_EXPORT" = false ]; then
        log "  Running sway export..."
        if ! timeout $TIMEOUT sway export > /dev/null 2>&1; then
            fail "$dir: sway export failed"
        fi
    fi

    log "  Running sway run..."
    OUTPUT=$(timeout $TIMEOUT sway run app.py 2>&1) || true

    echo ""
    echo "  Output: ${OUTPUT:0:200}"

    if echo "$OUTPUT" | grep -qi "error\|panic\|fatal"; then
        fail "$dir: Output contains errors"
    fi

    # Check each expected substring
    for exp in "${@:2}"; do
        if echo "$OUTPUT" | grep -q -- "$exp"; then
            pass "$dir: Found expected output '$exp'"
        else
            fail "$dir: Missing expected output '$exp'"
        fi
    done

    cd ~/tinycontainerruntime
}

# Run tests
START_TIME=$SECONDS
log "Starting integration tests..."

run_app "testapp2" "hello world" "Sum: 15" "Mean: 3.0"
run_app "testapp3" "hello world" "Sum: 15" "Mean: 3.0" "-117.0" "0.0"
run_app "testappscipy" "scipy test" "matmul: (100, 100)" "svd: u=(100, 100), s=(100,)" "fitted normal: mean="
run_app "testappplotting" "Sine Wave"

ELAPSED=$(( SECONDS - START_TIME ))
MINS=$(( ELAPSED / 60 ))
SECS=$(( ELAPSED % 60 ))

echo ""
if [ $MINS -gt 0 ]; then
    echo -e "${GREEN}All integration tests passed in ${MINS}m ${SECS}s${NC}"
else
    echo -e "${GREEN}All integration tests passed in ${SECS}s${NC}"
fi
