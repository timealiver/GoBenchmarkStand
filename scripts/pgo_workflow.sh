#!/usr/bin/env bash
# pgo_workflow.sh — full Profile-Guided Optimization pipeline.
#
# Two modes:
#   bench   — collect profile from go test -cpuprofile (reproducible, no server needed)
#   server  — collect profile from a live server under vegeta load (realistic)
#
# Usage:
#   bash scripts/pgo_workflow.sh bench          # recommended for report reproduction
#   bash scripts/pgo_workflow.sh server         # production-realistic profile
#   bash scripts/pgo_workflow.sh bench compare  # bench mode + benchstat comparison
set -euo pipefail

MODE="${1:-bench}"
EXTRA="${2:-}"
RESULTS_DIR="${RESULTS_DIR:-results}"
PGO_DIR="pgo"
PGO_PROFILE="$PGO_DIR/cpu.prof"
SERVER_ADDR="${SERVER_ADDR:-http://localhost:8080}"

mkdir -p "$RESULTS_DIR" bin

# ─────────────────────────────────────────────────────────────
# STEP 1: baseline benchmarks (no PGO)
# ─────────────────────────────────────────────────────────────
echo "=== [1/5] Building baseline binary (no PGO) ==="
go build -o bin/gostand_nopgo ./cmd/server
echo "    bin/gostand_nopgo built"

echo
echo "=== [2/5] Running baseline benchmarks ==="
go test ./bench/ \
    -bench=BenchmarkHandlers \
    -benchmem \
    -count=10 \
    -timeout=600s \
    > "$RESULTS_DIR/bench_nopgo.txt"
echo "    saved → $RESULTS_DIR/bench_nopgo.txt"

# ─────────────────────────────────────────────────────────────
# STEP 2: collect CPU profile
# ─────────────────────────────────────────────────────────────
echo
echo "=== [3/5] Collecting CPU profile (mode: $MODE) ==="

if [[ "$MODE" == "server" ]]; then
    echo "    Starting server in background…"
    ./bin/gostand_nopgo -addr :8080 -out "$RESULTS_DIR" -version nopgo &
    SERVER_PID=$!
    trap "kill $SERVER_PID 2>/dev/null || true" EXIT

    echo "    Warming up (30s vegeta @ 1000 req/s)…"
    vegeta attack \
        -rate=1000 \
        -duration=30s \
        -targets=<(printf "POST %s/v2/aggregate\nContent-Type: application/json\n@bench/payloads/payload_10k.json\n" "$SERVER_ADDR") \
        > /dev/null

    echo "    Collecting /debug/pprof/profile for 60s…"
    curl -sS -o "$PGO_PROFILE" \
        "$SERVER_ADDR/debug/pprof/profile?seconds=60" &
    CURL_PID=$!

    echo "    Sustaining load during profiling…"
    vegeta attack \
        -rate=1000 \
        -duration=65s \
        -targets=<(printf "POST %s/v2/aggregate\nContent-Type: application/json\n@bench/payloads/payload_10k.json\n" "$SERVER_ADDR") \
        > /dev/null

    wait $CURL_PID
    kill $SERVER_PID 2>/dev/null || true

else
    # bench mode: use go test -cpuprofile (reproducible, no server required)
    echo "    Running V2_Pools benchmark with -cpuprofile…"
    go test ./bench/ \
        -bench="BenchmarkHandlers/V2_Pools" \
        -benchmem \
        -count=5 \
        -cpuprofile="$PGO_PROFILE" \
        -timeout=300s \
        > /dev/null
    echo "    Profile size: $(wc -c < "$PGO_PROFILE") bytes"
fi

echo "    Saved CPU profile → $PGO_PROFILE"

# ─────────────────────────────────────────────────────────────
# STEP 3: rebuild with PGO
# ─────────────────────────────────────────────────────────────
echo
echo "=== [4/5] Building PGO binary ==="

# Go picks up default.pgo automatically from the main package directory.
cp "$PGO_PROFILE" cmd/server/default.pgo

go build -o bin/gostand_pgo ./cmd/server

# Verify PGO was applied.
PGO_FLAG=$(go version -m bin/gostand_pgo | grep pgo || echo "(not found)")
echo "    PGO flag in binary: $PGO_FLAG"

# Remove default.pgo so future plain builds are unaffected.
rm -f cmd/server/default.pgo

# ─────────────────────────────────────────────────────────────
# STEP 4: PGO benchmarks
# ─────────────────────────────────────────────────────────────
echo
echo "=== [5/5] Running PGO benchmarks ==="

# Temporarily place default.pgo so go test picks it up.
cp "$PGO_PROFILE" cmd/server/default.pgo

go test ./bench/ \
    -bench=BenchmarkHandlers \
    -benchmem \
    -count=10 \
    -timeout=600s \
    > "$RESULTS_DIR/bench_pgo.txt"

rm -f cmd/server/default.pgo
echo "    saved → $RESULTS_DIR/bench_pgo.txt"

# ─────────────────────────────────────────────────────────────
# STEP 5 (optional): benchstat comparison
# ─────────────────────────────────────────────────────────────
if [[ "$EXTRA" == "compare" ]] || command -v benchstat &>/dev/null; then
    echo
    echo "=== benchstat comparison (nopgo vs pgo) ==="
    if command -v benchstat &>/dev/null; then
        benchstat "$RESULTS_DIR/bench_nopgo.txt" "$RESULTS_DIR/bench_pgo.txt"
    else
        echo "    benchstat not installed. Install with:"
        echo "    go install golang.org/x/perf/cmd/benchstat@latest"
        echo
        echo "    Raw files saved:"
        echo "      $RESULTS_DIR/bench_nopgo.txt"
        echo "      $RESULTS_DIR/bench_pgo.txt"
    fi
fi

echo
echo "=== Done ==="
echo "  bin/gostand_nopgo  — binary without PGO"
echo "  bin/gostand_pgo    — binary with PGO (profile: $MODE)"
echo "  $RESULTS_DIR/bench_nopgo.txt"
echo "  $RESULTS_DIR/bench_pgo.txt"
