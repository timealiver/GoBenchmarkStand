# pgo_workflow.ps1 — Profile-Guided Optimization pipeline for Windows PowerShell.
#
# Usage:
#   .\scripts\pgo_workflow.ps1            # bench mode (default)
#   .\scripts\pgo_workflow.ps1 -Mode server -ServerAddr http://localhost:8080
#   .\scripts\pgo_workflow.ps1 -Compare   # run benchstat after
#
# Prerequisites: Go 1.21+, benchstat (optional)
#   go install golang.org/x/perf/cmd/benchstat@latest
param(
    [ValidateSet("bench","server")]
    [string]$Mode = "bench",

    [string]$ServerAddr = "http://localhost:8080",
    [string]$ResultsDir = "results",
    [string]$PgoDir     = "pgo",
    [switch]$Compare
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$PgoProfile = Join-Path $PgoDir "cpu.prof"

New-Item -ItemType Directory -Force -Path $ResultsDir | Out-Null
New-Item -ItemType Directory -Force -Path "bin"       | Out-Null

# ─────────────────────────────────────────────────────────────
# 1. Baseline benchmarks (no PGO)
# ─────────────────────────────────────────────────────────────
Write-Host "`n=== [1/5] Building baseline binary (no PGO) ===" -ForegroundColor Cyan
go build -o bin/gostand_nopgo.exe ./cmd/server
Write-Host "    bin/gostand_nopgo.exe built"

Write-Host "`n=== [2/5] Running baseline benchmarks ===" -ForegroundColor Cyan
$nopgoOut = Join-Path $ResultsDir "bench_nopgo.txt"
go test ./bench/ -bench=BenchmarkHandlers -benchmem -count=10 -timeout=600s |
    Tee-Object -FilePath $nopgoOut
Write-Host "    saved -> $nopgoOut"

# ─────────────────────────────────────────────────────────────
# 2. Collect CPU profile
# ─────────────────────────────────────────────────────────────
Write-Host "`n=== [3/5] Collecting CPU profile (mode: $Mode) ===" -ForegroundColor Cyan

if ($Mode -eq "server") {
    Write-Host "    Starting server…"
    $srv = Start-Process -FilePath "bin/gostand_nopgo.exe" `
        -ArgumentList "-addr", ":8080", "-out", $ResultsDir, "-version", "nopgo" `
        -PassThru -NoNewWindow
    Start-Sleep -Seconds 3

    Write-Host "    Warming up 30s…"
    # Requires vegeta in PATH
    $target = "POST $ServerAddr/v2/aggregate`nContent-Type: application/json`n@bench/payloads/payload_10k.json"
    $target | vegeta attack -rate=500 -duration=30s | Out-Null

    Write-Host "    Collecting profile (60s)…"
    Invoke-WebRequest -Uri "$ServerAddr/debug/pprof/profile?seconds=60" `
        -OutFile $PgoProfile
    Write-Host "    Collected."

    $srv | Stop-Process -Force -ErrorAction SilentlyContinue
} else {
    Write-Host "    Running V2_Pools benchmark with -cpuprofile…"
    go test ./bench/ `
        -bench="BenchmarkHandlers/V2_Pools" `
        -benchmem `
        -count=5 `
        -cpuprofile=$PgoProfile `
        -timeout=300s | Out-Null

    $profileSize = (Get-Item $PgoProfile).Length
    Write-Host "    Profile size: $profileSize bytes"
}

Write-Host "    Saved CPU profile -> $PgoProfile"

# ─────────────────────────────────────────────────────────────
# 3. Rebuild with PGO
# ─────────────────────────────────────────────────────────────
Write-Host "`n=== [4/5] Building PGO binary ===" -ForegroundColor Cyan

$defaultPgo = Join-Path "cmd" "server" "default.pgo"
Copy-Item $PgoProfile $defaultPgo -Force

go build -o bin/gostand_pgo.exe ./cmd/server

# Verify
$pgoFlag = go version -m bin/gostand_pgo.exe | Select-String "pgo"
Write-Host "    PGO flag in binary: $pgoFlag"

Remove-Item $defaultPgo -Force -ErrorAction SilentlyContinue

# ─────────────────────────────────────────────────────────────
# 4. PGO benchmarks
# ─────────────────────────────────────────────────────────────
Write-Host "`n=== [5/5] Running PGO benchmarks ===" -ForegroundColor Cyan

Copy-Item $PgoProfile $defaultPgo -Force

$pgoOut = Join-Path $ResultsDir "bench_pgo.txt"
go test ./bench/ -bench=BenchmarkHandlers -benchmem -count=10 -timeout=600s |
    Tee-Object -FilePath $pgoOut

Remove-Item $defaultPgo -Force -ErrorAction SilentlyContinue
Write-Host "    saved -> $pgoOut"

# ─────────────────────────────────────────────────────────────
# 5. benchstat comparison (optional)
# ─────────────────────────────────────────────────────────────
if ($Compare) {
    Write-Host "`n=== benchstat comparison (nopgo vs pgo) ===" -ForegroundColor Cyan
    if (Get-Command benchstat -ErrorAction SilentlyContinue) {
        benchstat $nopgoOut $pgoOut
    } else {
        Write-Host "    benchstat not installed. Run:"
        Write-Host "    go install golang.org/x/perf/cmd/benchstat@latest"
    }
}

Write-Host "`n=== Done ===" -ForegroundColor Green
Write-Host "  bin/gostand_nopgo.exe  — без PGO"
Write-Host "  bin/gostand_pgo.exe    — с PGO (профиль: $Mode)"
Write-Host "  $nopgoOut"
Write-Host "  $pgoOut"
