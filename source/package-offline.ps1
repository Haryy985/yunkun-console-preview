# Build a data-free release candidate from a reviewed allowlist.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+-offline$')][string]$Version,
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath($PSScriptRoot)
$binary = Join-Path $root 'bin\yunkun.exe'
$actual = (& $binary -version).Trim()
if ($LASTEXITCODE -ne 0 -or $actual -ne $Version) {
    throw "Binary version $actual does not match package version $Version."
}

$output = if ($OutputDirectory) {
    [IO.Path]::GetFullPath($OutputDirectory)
} else {
    Join-Path $root '.tools\release-candidates'
}
$name = "yunkun-$Version"
$stage = Join-Path $output $name
$archive = Join-Path $output "$name-windows-x64.zip"
if ((Test-Path -LiteralPath $stage) -or (Test-Path -LiteralPath $archive)) {
    throw 'Candidate already exists; no existing package was replaced.'
}

foreach ($required in @(
    'runtime\pgsql-18.4\bin\pg_ctl.exe',
    'runtime\pgsql-18.4\bin\initdb.exe',
    'runtime\pgsql-18.4\bin\psql.exe',
    'setup-and-start-offline.bat',
    'setup-and-start-offline.ps1',
    'stop-offline.bat',
    'upgrade-offline.ps1'
)) {
    if (-not (Test-Path -LiteralPath (Join-Path $root $required) -PathType Leaf)) {
        throw "Missing release dependency: $required"
    }
}

New-Item -ItemType Directory -Path (Join-Path $stage 'bin'), (Join-Path $stage 'docs'),
    (Join-Path $stage 'screenshots'), (Join-Path $stage 'skills') -Force | Out-Null
foreach ($file in @(
    'bin\yunkun.exe', 'setup-and-start-offline.bat', 'setup-and-start-offline.ps1',
    'stop-offline.bat', 'upgrade-offline.ps1', 'README.md', 'CHANGELOG.md',
    'NOTICE.md', 'LICENSE', 'config.example.json',
    'docs\README.md', 'docs\OFFLINE_UPDATE.md', 'docs\DEPLOYMENT.md',
    'docs\WINDOWS.md', 'docs\agent-reliability-2026-10-16.md',
    'screenshots\dashboard.png', 'screenshots\llm.png', 'screenshots\proxy-pools.png',
    'screenshots\rag.png', 'screenshots\settings-dark.png'
)) {
    $source = Join-Path $root $file
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { throw "Missing release file: $file" }
    if ((Get-Item -LiteralPath $source).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw "Linked release file not allowed: $file"
    }
    $destination = Join-Path $stage $file
    Copy-Item -LiteralPath $source -Destination $destination -ErrorAction Stop
}
$runtime = Join-Path $root 'runtime\pgsql-18.4'
if (@(Get-ChildItem -LiteralPath $runtime -Recurse -Force -Attributes ReparsePoint).Count) {
    throw 'Linked runtime files are not allowed.'
}
New-Item -ItemType Directory -Path (Join-Path $stage 'runtime') -Force | Out-Null
Copy-Item -LiteralPath $runtime -Destination (Join-Path $stage 'runtime') -Recurse -ErrorAction Stop

Compress-Archive -LiteralPath $stage -DestinationPath $archive -CompressionLevel Optimal -ErrorAction Stop
Add-Type -AssemblyName System.IO.Compression
$zip = [IO.Compression.ZipFile]::OpenRead($archive)
try {
    $entries = @($zip.Entries | ForEach-Object { $_.FullName.Replace('\', '/') })
    foreach ($required in @(
        "$name/bin/yunkun.exe", "$name/runtime/pgsql-18.4/bin/pg_ctl.exe",
        "$name/upgrade-offline.ps1"
    )) {
        if ($entries -notcontains $required) { throw "Release ZIP lacks $required" }
    }
    if (@($entries | Where-Object { $_ -match '(^|/)(\.yunkun-local[^/]*|\.tools|config\.json|installation\.json)(/|$)' }).Count) {
        throw 'Private installation data detected in ZIP.'
    }
} finally {
    $zip.Dispose()
}
$hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
"$hash  $([IO.Path]::GetFileName($archive))" |
    Set-Content -LiteralPath (Join-Path $output 'SHA256SUMS.txt') -Encoding ascii
Write-Host "Candidate: $archive"
Write-Host "SHA-256: $hash"
Write-Host 'No customer installation directory was read or included.'
