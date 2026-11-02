param([string]$OutputDirectory = '')
$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
$metadata = Get-Content -LiteralPath (Join-Path $project 'testdata/real/source.json') -Raw -Encoding utf8 | ConvertFrom-Json
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $project 'testdata/real' }
$directory = [System.IO.Path]::GetFullPath($OutputDirectory)
[System.IO.Directory]::CreateDirectory($directory) | Out-Null
$target = Join-Path $directory 'top-100k-domains.csv'
function Assert-Source([string]$Path) {
    if ((Get-Item -LiteralPath $Path).Length -ne $metadata.bytes) { throw 'Source size mismatch' }
    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ine $metadata.sha256) { throw 'Source SHA256 mismatch' }
}
if (Test-Path -LiteralPath $target) {
    Assert-Source $target
} else {
    $temporary = Join-Path ([System.IO.Path]::GetTempPath()) ('powerwhois-data-' + [guid]::NewGuid().ToString('N') + '.download')
    try {
        Invoke-WebRequest -Uri $metadata.url -OutFile $temporary -TimeoutSec 60
        Assert-Source $temporary
        [System.IO.File]::Copy($temporary, $target, $false)
    } finally {
        if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -ErrorAction SilentlyContinue }
    }
}
[ordered]@{ file = $target; bytes = $metadata.bytes; sha256 = $metadata.sha256; commit = $metadata.commit } | ConvertTo-Json
