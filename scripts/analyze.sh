#!/usr/bin/env bash
# analyze.sh — summarise vegeta results from results/ into a comparison table.
#
# Requires: jq, column
# Usage:
#   bash scripts/analyze.sh [results_dir]
set -euo pipefail

RESULTS_DIR="${1:-results}"

if ! command -v jq &>/dev/null; then
    echo "ERROR: jq is required (brew install jq / apt install jq)" >&2
    exit 1
fi

echo "=== GoStand — Latency Summary (nanoseconds) ==="
printf "%-10s %-8s %10s %10s %10s %10s %10s %10s\n" \
    "Version" "Rate" "Mean" "P50" "P90" "P95" "P99" "Max"
echo "$(printf '%0.s-' {1..78})"

for JSON in "$RESULTS_DIR"/*.json; do
    [[ -f "$JSON" ]] || continue
    BASE=$(basename "$JSON" .json)
    VERSION="${BASE%%_*}"
    RATE="${BASE#*_}"

    MEAN=$(jq '.latencies.mean'  "$JSON" 2>/dev/null || echo 0)
    P50=$(jq  '.latencies."50th"' "$JSON" 2>/dev/null || echo 0)
    P90=$(jq  '.latencies."90th"' "$JSON" 2>/dev/null || echo 0)
    P95=$(jq  '.latencies."95th"' "$JSON" 2>/dev/null || echo 0)
    P99=$(jq  '.latencies."99th"' "$JSON" 2>/dev/null || echo 0)
    MAX=$(jq  '.latencies.max'   "$JSON" 2>/dev/null || echo 0)

    printf "%-10s %-8s %10d %10d %10d %10d %10d %10d\n" \
        "$VERSION" "$RATE" "$MEAN" "$P50" "$P90" "$P95" "$P99" "$MAX"
done

echo
echo "=== Success Rates ==="
printf "%-10s %-8s %10s %10s\n" "Version" "Rate" "Success%" "Req/s"
echo "$(printf '%0.s-' {1..42})"

for JSON in "$RESULTS_DIR"/*.json; do
    [[ -f "$JSON" ]] || continue
    BASE=$(basename "$JSON" .json)
    VERSION="${BASE%%_*}"
    RATE="${BASE#*_}"

    SUCCESS=$(jq '(.status_codes."200" // 0) / .requests * 100 | floor' "$JSON" 2>/dev/null || echo 0)
    THROUGHPUT=$(jq '.throughput | floor' "$JSON" 2>/dev/null || echo 0)

    printf "%-10s %-8s %9d%% %10d\n" "$VERSION" "$RATE" "$SUCCESS" "$THROUGHPUT"
done

echo
echo "=== GC Metrics (from JSONL snapshots) ==="
echo "Version  | AvgHeapInuse (MB) | AvgNumGC | AvgLastPause (µs)"
echo "---------|-------------------|----------|-----------------"

for JSONL in "$RESULTS_DIR"/metrics_*.jsonl; do
    [[ -f "$JSONL" ]] || continue
    VERSION=$(basename "$JSONL" .jsonl | sed 's/metrics_//')
    jq -s '
        {
            version: .[0].version,
            avg_heap_mb: (map(.heap_inuse_bytes) | add / length / 1048576 | floor),
            avg_num_gc:  (map(.num_gc) | max),
            avg_pause_us: (map(.last_pause_ns) | add / length / 1000 | floor)
        }
        | "\(.version)  | \(.avg_heap_mb) MB  | \(.avg_num_gc) | \(.avg_pause_us) µs"
    ' "$JSONL" 2>/dev/null || echo "$VERSION | n/a"
done

echo
echo "Hint: view HDR histograms with:"
echo "  go tool pprof results/v1_1000rps.hdr"
echo "Or plot with HdrHistogram Plotter: https://hdrhistogram.github.io/HdrHistogram/plotFiles.html"
