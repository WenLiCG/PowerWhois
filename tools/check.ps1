param([switch]$Race)
$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
$taskRoot = Split-Path -Parent $project
$portable = Join-Path $taskRoot '.tools/go/bin/go.exe'
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go -and (Test-Path -LiteralPath $portable)) {
    $go = $portable
    $env:GOCACHE = Join-Path $taskRoot '.tools/gocache'
    $env:GOMODCACHE = Join-Path $taskRoot '.tools/gomod'
    $env:GOPATH = Join-Path $taskRoot '.tools/gopath'
}
if (-not $go) { throw 'Install Go >= 1.26 or put it in PATH.' }
Push-Location (Join-Path $project 'src/backend')
try {
    $testArgs = @('test','./...','-count=1')
    if ($Race) { $testArgs = @('test','-race','./...','-count=1') }
    & $go @testArgs
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & $go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    New-Item -ItemType Directory -Path (Join-Path $project 'bin') -Force | Out-Null
    & $go build -o (Join-Path $project 'bin/powerwhois.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
} finally { Pop-Location }
Push-Location (Join-Path $project 'src/frontend')
try {
    & npm.cmd test
    if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' }
    & npm.cmd run lint
    if ($LASTEXITCODE -ne 0) { throw 'Frontend lint failed' }
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
} finally { Pop-Location }
