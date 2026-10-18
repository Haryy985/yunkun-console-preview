[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$BinaryPath,
    [string]$OutputDirectory = 'F:\YunkunOffline\v1.0.3-offline-complete-20261018',
    [string]$BaselineDirectory = 'F:\AI\ARTEX-main(1)\.tools\publish-v1.0.2-20261016-final\yunkun-v1.0.2-offline'
)
$ErrorActionPreference = 'Stop'
$sourceRoot = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$baseline = [IO.Path]::GetFullPath($BaselineDirectory)
$output = [IO.Path]::GetFullPath($OutputDirectory)
$binary = (Get-Item -LiteralPath $BinaryPath).FullName
if ((& $binary -version).Trim() -ne 'v1.0.3-offline') { throw 'Unexpected binary version.' }
$stage = Join-Path $output 'yunkun-v1.0.3-offline'
if (Test-Path -LiteralPath $stage) { throw 'Candidate already exists; nothing overwritten.' }
New-Item -ItemType Directory -Path $stage | Out-Null
foreach ($dir in @('bin', 'runtime', 'docs', 'skills', 'screenshots')) {
    New-Item -ItemType Directory -Path (Join-Path $stage $dir) | Out-Null
}

function Copy-ReleaseFile([string]$FromRoot, [string]$Relative, [string]$DestinationRelative = '') {
    if (-not $DestinationRelative) { $DestinationRelative = $Relative }
    $from = Join-Path $FromRoot $Relative
    $item = Get-Item -LiteralPath $from -ErrorAction Stop
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Linked release file: $Relative" }
    $to = Join-Path $stage $DestinationRelative
    New-Item -ItemType Directory -Path (Split-Path -Parent $to) -Force | Out-Null
    Copy-Item -LiteralPath $from -Destination $to
}

Copy-Item -LiteralPath $binary -Destination (Join-Path $stage 'bin\yunkun.exe')
foreach ($file in @('setup-and-start-offline.bat','setup-and-start-offline.ps1',
    'stop-offline.bat','upgrade-offline.ps1','config.example.json',
    'docs\OFFLINE_UPDATE.md','docs\RELEASE_1.0.3.md','docs\RELEASE_1.0.3_ACCEPTANCE.md',
    'docs\AGENT_SKILL_GO_PORT.md')) {
    Copy-ReleaseFile $sourceRoot $file
}
foreach ($file in @('LICENSE','NOTICE.md')) { Copy-ReleaseFile $baseline $file }
Copy-ReleaseFile $sourceRoot 'docs\OFFLINE_README.md' 'README.md'
Copy-ReleaseFile $sourceRoot 'docs\RELEASE_1.0.3.md' 'CHANGELOG.md'

$runtime = Join-Path $baseline 'runtime\pgsql-18.4'
if (-not (Test-Path -LiteralPath (Join-Path $runtime 'bin\postgres.exe'))) { throw 'Missing PostgreSQL runtime.' }
if (@(Get-ChildItem -LiteralPath $runtime -Recurse -Force -Attributes ReparsePoint).Count) {
    throw 'Linked runtime files are not allowed.'
}
Copy-Item -LiteralPath $runtime -Destination (Join-Path $stage 'runtime') -Recurse
foreach ($name in @('dashboard.png','rag.png','proxy-pools.png')) {
    Copy-ReleaseFile $baseline ("screenshots\" + $name)
}
# Keep existing optional public skills. Never copy the original account/MCP/hook configuration.
foreach ($name in @('api-recon','playwright-cli','scopesentry','yunkun-security-workflow')) {
    $root = Join-Path $sourceRoot ("skills\" + $name)
    foreach ($file in Get-ChildItem -LiteralPath $root -Recurse -File) {
        if ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Linked skill file.' }
        if ($file.Extension -notin @('.md','.py','.js','.json','.yaml','.yml','.txt','.sh')) {
            throw "Unreviewed skill file type: $($file.Name)"
        }
        $relative = $file.FullName.Substring($sourceRoot.Length + 1)
        Copy-ReleaseFile $sourceRoot $relative
    }
}
$referenceRoot = Join-Path $sourceRoot 'skills\clown-src-6k-skill-2-1'
foreach ($category in @('rules','references','skills','templates')) {
    foreach ($file in Get-ChildItem -LiteralPath (Join-Path $referenceRoot $category) -Filter '*.md' -Recurse -File) {
        if ($file.Name -eq 'AGENTS.md') { continue }
        $relative = $file.FullName.Substring($referenceRoot.Length + 1)
        Copy-ReleaseFile $referenceRoot $relative ("skills\yunkun-security-workflow\references\" + $relative)
    }
}
$skillReadme = Join-Path $stage 'skills\yunkun-security-workflow\references\README.md'
[IO.File]::WriteAllText($skillReadme,
    "Reference material adapted from the deployment owner's Clown SRC 6K Skill 2.1. Consult only relevant sections. Native runtime behavior and task authorization take precedence. Original hooks, coordinator settings and provider accounts are not included.`r`n",
    [Text.UTF8Encoding]::new($false))

$sourceFiles = foreach ($dir in @('agent','cmd','config','db','enrich','evidence','guard','intercept',
    'llmpool','llmrec','mcphttp','migration','notify','proxypool','rag','report','selfupdate','server','sidequestion','traffic')) {
    Get-ChildItem -LiteralPath (Join-Path $sourceRoot $dir) -Recurse -File |
        Where-Object { $_.FullName -notmatch '\\(webui|data|\.tools)\\' -and $_.Extension -in @('.go','.sql') }
}
$sourceFiles += @(Get-Item -LiteralPath (Join-Path $sourceRoot 'go.mod'),(Join-Path $sourceRoot 'go.sum'))
$sourceFiles += @(Get-ChildItem -LiteralPath (Join-Path $sourceRoot 'web\src') -Recurse -File)
foreach ($relative in @('scripts','tests','web\public','.github\workflows')) {
    $sourceFiles += @(Get-ChildItem -LiteralPath (Join-Path $sourceRoot $relative) -Recurse -File)
}
foreach ($relative in @('build.sh','config.example.json','setup-and-start-offline.bat',
    'setup-and-start-offline.ps1','stop-offline.bat','upgrade-offline.ps1',
    'web\package.json','web\package-lock.json','web\next.config.ts','web\tsconfig.json')) {
    $path = Join-Path $sourceRoot $relative
    if (Test-Path -LiteralPath $path -PathType Leaf) { $sourceFiles += Get-Item -LiteralPath $path }
}
foreach ($name in @('api-recon','playwright-cli','scopesentry','yunkun-security-workflow')) {
    $sourceFiles += @(Get-ChildItem -LiteralPath (Join-Path $sourceRoot ("skills\" + $name)) -Recurse -File)
}
foreach ($category in @('rules','references','skills','templates')) {
    $sourceFiles += @(Get-ChildItem -LiteralPath (Join-Path $referenceRoot $category) -Filter '*.md' -Recurse -File |
        Where-Object Name -ne 'AGENTS.md')
}
$sourceFiles | Sort-Object FullName -Unique | ForEach-Object {
    [pscustomobject]@{ path=$_.FullName.Substring($sourceRoot.Length+1).Replace('\','/'); sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash }
} | Export-Csv -LiteralPath (Join-Path $stage 'docs\SOURCE_MANIFEST.csv') -NoTypeInformation -Encoding utf8

$private = @(Get-ChildItem -LiteralPath $stage -Recurse -Force -File | Where-Object {
    $_.Name -match '^(config\.json|jwt\.key|proxy-pool\.key|installation\.json|fofa_accounts\.md)$' -or
    $_.Extension -in @('.db','.log','.pem','.key','.p12','.pfx') -or
    $_.FullName -match '\\(\.yunkun-local|\.tools|postgres-data|node_modules|\.next)\\'
})
if ($private.Count) { throw 'Private or generated files detected in candidate.' }
$manifest = @(Get-ChildItem -LiteralPath $stage -Recurse -File | Sort-Object FullName | ForEach-Object {
    [pscustomobject]@{ path=$_.FullName.Substring($stage.Length+1).Replace('\','/'); bytes=$_.Length; sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash }
})
$manifest | Export-Csv -LiteralPath (Join-Path $stage 'docs\PACKAGE_MANIFEST.csv') -NoTypeInformation -Encoding utf8
Write-Host "Staged candidate: $stage"
Write-Host 'Archive only after isolated installation and upgrade verification, using an untouched copy.'
