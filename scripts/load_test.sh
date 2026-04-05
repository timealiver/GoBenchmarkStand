#!/usr/bin/env bash
# load_test.sh — run vegeta load tests against all four handler versions.
#
# Prerequisites:
#   - Server running: go run ./cmd/server -addr :8080 -out results -version all
#   - vegeta installed: https://github.com/tsenart/vegeta
#   - Payloads generated: go run ./bench/gen -out bench/payloads
#
# Usage:
#   bash scripts/load_test.sh [payload_file] [rates...] [duration]
#
# Examples:
#   bash scripts/load_test.sh                          # defaults
#   bash scripts/load_test.sh bench/payloads/payload_10k.json 500 1000 2000 5000 60s
set -euo pipefail

SERVER="${SERVER:-http://localhost:8080}"
PAYLOAD="${1:-bench/payloads/payload_10k.json}"
DURATION="${DURATION:-60s}"
RESULTS_DIR="${RESULTS_DIR:-results}"
VERSIONS=(v1 v2 v3 v4)

# Remaining args after $1 are treated as rates; default if not given.
shift 1 2>/dev/null || true
if [[ $# -gt 0 ]]; then
    RATES=("$@")
else
    RATES=(500 1000 2000 5000)
fi

mkdir -p "$RESULTS_DIR"

echo "=== GoStand Load Test ==="
echo "Payload : $PAYLOAD ($(wc -c < "$PAYLOAD") bytes)"
echo "Rates   : ${RATES[*]} req/s"
echo "Duration: $DURATION"
echo "Server  : $SERVER"
echo

for VERSION in "${VERSIONS[@]}"; do
    ENDPOINT="$SERVER/$VERSION/aggregate"
    TARGETS_FILE=$(mktemp)

    # vegeta targets format: METHOD URL\nContent-Type: ...\n@body_file\n
    cat > "$TARGETS_FILE" <<EOF
POST $ENDPOINT
Content-Type: application/json
@$PAYLOAD
EOF

    for RATE in "${RATES[@]}"; do
        OUT_BASE="$RESULTS_DIR/${VERSION}_${RATE}rps"
        echo "▶ $VERSION  @ ${RATE} req/s  → $OUT_BASE.*"

        vegeta attack \
            -rate="$RATE" \
            -duration="$DURATION" \
            -targets="$TARGETS_FILE" \
            -keepalive=true \
            -max-connections=512 \
        | tee "${OUT_BASE}.bin" \
        | vegeta report -type=json > "${OUT_BASE}.json"

        # Human-readable summary.
        vegeta report -type=text < "${OUT_BASE}.bin" > "${OUT_BASE}.txt"

        # HDR histogram for p99/p99.9 tail analysis.
        vegeta report -type=hdrplot < "${OUT_BASE}.bin" > "${OUT_BASE}.hdr"

        echo "  done: $(grep -o '"99th":[^,}]*' "${OUT_BASE}.json" || echo '?') (p99)"
    done

    rm -f "$TARGETS_FILE"
done

echo
echo "=== All done. Results in $RESULTS_DIR/ ==="
echo "Run: bash scripts/analyze.sh  to generate comparison table"
