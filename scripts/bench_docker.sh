#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

IMAGE="${GOSTAND_BENCH_IMAGE:-gostand-bench}"
OUT="${1:-$ROOT/results/bench_linux.txt}"

mkdir -p "$(dirname "$OUT")"

echo "Building Docker image $IMAGE ..."
docker build -t "$IMAGE" -f Dockerfile "$ROOT"

echo "Running benchmarks inside Linux container -> $OUT"
docker run --rm "$IMAGE" | tee "$OUT"

echo "Done. Compare with a Windows run, e.g.:"
echo "  go run ./bench/report/ -in results/bench_windows.txt -cmp $OUT -out results/report.html"
