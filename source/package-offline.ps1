# Build a data-free release candidate from a reviewed allowlist.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+-offline$')][string]$Version,
    [string]$OutputDirectory,
    [string]$PostgresRuntimeDirectory,
    [string]$VCRedistFile,
    [string]$BinaryFile
)

$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath($PSScriptRoot)
$binary = if ($BinaryFile) { [IO.Path]::GetFullPath($BinaryFile) } else { Join-Path $root 'bin\yunkun.exe' }
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
$archive = Join-Path $output ("yunkun-" + $Version.TrimStart('v') + "-windows-amd64.zip")
$postgres = if ($PostgresRuntimeDirectory) {
    [IO.Path]::GetFullPath($PostgresRuntimeDirectory)
} else {
    Join-Path $root 'runtime\pgsql-18.4'
}
$vc = if ($VCRedistFile) { [IO.Path]::GetFullPath($VCRedistFile) } else { Join-Path $root 'runtime\vc_redist.x64.exe' }
if ((Test-Path -LiteralPath $stage) -or (Test-Path -LiteralPath $archive)) {
    throw 'Candidate already exists; no existing package was replaced.'
}

foreach ($required in @(
    'setup-and-start-offline.bat',
    'setup-and-start-offline.ps1',
    'stop-offline.bat',
    'upgrade-offline.ps1'
)) {
    if (-not (Test-Path -LiteralPath (Join-Path $root $required) -PathType Leaf)) {
        throw "Missing release dependency: $required"
    }
}
foreach ($required in @('bin\pg_ctl.exe', 'bin\initdb.exe', 'bin\psql.exe')) {
    if (-not (Test-Path -LiteralPath (Join-Path $postgres $required) -PathType Leaf)) {
        throw "Missing PostgreSQL release dependency: $required"
    }
}
if (-not (Test-Path -LiteralPath $vc -PathType Leaf)) {
    throw 'Missing VC++ runtime installer.'
}
if ((Get-Item -LiteralPath $vc).Attributes -band [IO.FileAttributes]::ReparsePoint) {
    throw 'Linked VC++ runtime installer not allowed.'
}

New-Item -ItemType Directory -Path (Join-Path $stage 'bin'), (Join-Path $stage 'docs'),
    (Join-Path $stage 'screenshots'), (Join-Path $stage 'skills') -Force | Out-Null
foreach ($file in @(
    'bin\yunkun.exe', 'setup-and-start-offline.bat', 'setup-and-start-offline.ps1',
    'stop-offline.bat', 'upgrade-offline.ps1', 'README.md', 'CHANGELOG.md',
    'NOTICE.md', 'LICENSE', 'config.example.json',
    'docs\README.md', 'docs\OFFLINE_UPDATE.md', 'docs\RELEASE_1.1.0.md', 'docs\RELEASE_1.1.2.md', 'docs\DEPLOYMENT.md',
    'docs\WINDOWS.md', 'docs\agent-reliability-2026-10-16.md',
    'screenshots\dashboard.png', 'screenshots\llm.png', 'screenshots\proxy-pools.png',
    'screenshots\rag.png', 'screenshots\settings-dark.png'
)) {
    $source = if ($file -eq 'bin\yunkun.exe') { $binary } else { Join-Path $root $file }
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { throw "Missing release file: $file" }
    if ((Get-Item -LiteralPath $source).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw "Linked release file not allowed: $file"
    }
    $destination = Join-Path $stage $file
    Copy-Item -LiteralPath $source -Destination $destination -ErrorAction Stop
}
if (@(Get-ChildItem -LiteralPath $postgres -Recurse -Force -Attributes ReparsePoint).Count) {
    throw 'Linked runtime files are not allowed.'
}
if (@(Get-ChildItem -LiteralPath (Join-Path $root 'skills') -Recurse -Force -Attributes ReparsePoint).Count) {
    throw 'Linked skill files are not allowed.'
}
New-Item -ItemType Directory -Path (Join-Path $stage 'runtime') -Force | Out-Null
Copy-Item -LiteralPath $postgres -Destination (Join-Path $stage 'runtime') -Recurse -ErrorAction Stop
Copy-Item -LiteralPath $vc -Destination (Join-Path $stage 'runtime\vc_redist.x64.exe') -ErrorAction Stop
Copy-Item -Path (Join-Path $root 'skills\*') -Destination (Join-Path $stage 'skills') -Recurse -ErrorAction Stop

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$stream = [IO.File]::Open($archive, [IO.FileMode]::CreateNew)
try {
    $writer = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $true)
    try {
        foreach ($item in Get-ChildItem -LiteralPath $stage -File -Recurse -Force) {
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                throw "Linked release file not allowed: $($item.FullName)"
            }
            $relative = $item.FullName.Substring($stage.Length + 1).Replace('\', '/')
            $entry = $writer.CreateEntry("$name/$relative", [IO.Compression.CompressionLevel]::Optimal)
            $sourceStream = [IO.File]::OpenRead($item.FullName)
            try {
                $targetStream = $entry.Open()
                try { $sourceStream.CopyTo($targetStream) } finally { $targetStream.Dispose() }
            } finally { $sourceStream.Dispose() }
        }
    } finally { $writer.Dispose() }
} finally { $stream.Dispose() }
$zip = [IO.Compression.ZipFile]::OpenRead($archive)
try {
    $entries = @($zip.Entries | ForEach-Object { $_.FullName.Replace('\', '/') })
    foreach ($required in @(
        "$name/bin/yunkun.exe", "$name/runtime/pgsql-18.4/bin/pg_ctl.exe",
        "$name/runtime/vc_redist.x64.exe",
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
    Set-Content -LiteralPath (Join-Path $output 'SHA256SUMS') -Encoding ascii
Write-Host "Candidate: $archive"
Write-Host "SHA-256: $hash"
Write-Host 'No customer installation directory was read or included.'
