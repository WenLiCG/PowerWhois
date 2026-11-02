param(
    [Parameter(Mandatory=$true)][string]$File,
    [string]$Sheet = '',
    [string]$Column = 'B',
    [switch]$Header,
    [string]$Rules = 'omission,repetition,transposition',
    [int]$MaxCandidates = 500000,
    [string]$Url = 'http://127.0.0.1:8080'
)
$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $project 'bin/powerwhois.exe'
$importArgs = @('import','--url',$Url,'--file',$File,'--column',$Column)
if ($Sheet) { $importArgs += @('--sheet',$Sheet) }
if ($Header) { $importArgs += '--header' }
$job = & $exe @importArgs | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'Import failed' }
$generated = & $exe generate --url $Url --job $job.id --rules $Rules --max-candidates $MaxCandidates
if ($LASTEXITCODE -ne 0) { throw "Generation failed for job $($job.id)" }
& $exe start --url $Url --job $job.id
if ($LASTEXITCODE -ne 0) { throw "Start failed for job $($job.id)" }
