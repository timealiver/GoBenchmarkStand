$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$Image = if ($env:GOSTAND_BENCH_IMAGE) { $env:GOSTAND_BENCH_IMAGE } else { "gostand-bench" }
$Out = if ($args[0]) { $args[0] } else { Join-Path $Root "results\bench_linux.txt" }

New-Item -ItemType Directory -Force -Path (Split-Path $Out) | Out-Null

Write-Host "Building Docker image $Image ..."
docker build -t $Image -f Dockerfile $Root
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Running benchmarks inside Linux container -> $Out"
$output = docker run --rm $Image 2>&1
$dockerExit = $LASTEXITCODE
$output | Out-File -FilePath $Out -Encoding utf8
if ($dockerExit -ne 0) { exit $dockerExit }

Write-Host "Done ($Out). Last lines:"
Get-Content $Out -Tail 12

Write-Host ""
Write-Host "Example report:"
Write-Host "  go run ./bench/report/ -in results/bench_windows.txt -cmp $Out -out results/report.html"
