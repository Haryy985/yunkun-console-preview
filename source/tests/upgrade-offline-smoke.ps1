$ErrorActionPreference = 'Stop'
$testRoot = Join-Path ([IO.Path]::GetFullPath([IO.Path]::GetTempPath())) ("yunkun-upgrade-test-" + [Guid]::NewGuid().ToString('N'))
$old = Join-Path $testRoot 'old'
$new = Join-Path $testRoot 'new'
$backup = Join-Path $testRoot 'backup'

try {
    foreach ($path in @(
        $old, $new, (Join-Path $old '.yunkun-local\postgres-data'),
        (Join-Path $old 'skills\custom'), (Join-Path $old 'skills\default'),
        (Join-Path $old 'bin'), (Join-Path $new 'bin'),
        (Join-Path $new 'runtime\pgsql-18.4\bin'), (Join-Path $new 'skills\default')
    )) {
        New-Item -ItemType Directory -Path $path -Force | Out-Null
    }
    '{"format":1,"databasePort":55991}' | Set-Content -LiteralPath (Join-Path $old '.yunkun-local\installation.json')
    @{ skill_dir = (Join-Path $old 'skills'); database = @{ host = '127.0.0.1' } } |
        ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $old '.yunkun-local\config.json')
    '18' | Set-Content -LiteralPath (Join-Path $old '.yunkun-local\postgres-data\PG_VERSION')
    'test-user-data' | Set-Content -LiteralPath (Join-Path $old '.yunkun-local\postgres-data\record')
    [IO.File]::WriteAllText((Join-Path $old 'bin\jwt.key'), '0123456789abcdef0123456789abcdef')
    [IO.File]::WriteAllText((Join-Path $old 'bin\proxy-pool.key'), 'abcdef0123456789abcdef0123456789')
    'my skill' | Set-Content -LiteralPath (Join-Path $old 'skills\custom\SKILL.md')
    'user override' | Set-Content -LiteralPath (Join-Path $old 'skills\default\SKILL.md')
    'bundled skill' | Set-Content -LiteralPath (Join-Path $new 'skills\default\SKILL.md')
    '' | Set-Content -LiteralPath (Join-Path $new 'bin\yunkun.exe')
    '' | Set-Content -LiteralPath (Join-Path $new 'runtime\pgsql-18.4\bin\pg_ctl.exe')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot '..\upgrade-offline.ps1') -Destination $new
    '' | Set-Content -LiteralPath (Join-Path $new 'setup-and-start-offline.ps1')

    & (Join-Path $new 'upgrade-offline.ps1') -OldDirectory $old -BackupDirectory $backup
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed.' }
    foreach ($path in @(
        (Join-Path $backup '.yunkun-local\postgres-data\record'),
        (Join-Path $new '.yunkun-local\postgres-data\record'),
        (Join-Path $old '.yunkun-local\postgres-data\record'),
        (Join-Path $backup 'bin\jwt.key'),
        (Join-Path $new 'bin\jwt.key'),
        (Join-Path $old 'bin\jwt.key'),
        (Join-Path $new 'bin\proxy-pool.key'),
        (Join-Path $backup 'bin\proxy-pool.key'),
        (Join-Path $new '.yunkun-local\upgrade-bundled-skills\default\SKILL.md'),
        (Join-Path $new 'skills\custom\SKILL.md'),
        (Join-Path $new 'skills\default\SKILL.md')
    )) {
        if (-not (Test-Path -LiteralPath $path)) { throw "Missing retained data: $path" }
    }
    $config = Get-Content -LiteralPath (Join-Path $new '.yunkun-local\config.json') -Raw | ConvertFrom-Json
    if ($config.skill_dir -ne (Join-Path $new 'skills')) { throw 'Skill path was not updated.' }
    if ((Get-Content -LiteralPath (Join-Path $new 'skills\default\SKILL.md') -Raw).Trim() -ne 'user override') {
        throw 'User Skill changes were overwritten by the new package.'
    }
    if ((Get-Content -LiteralPath (Join-Path $new '.yunkun-local\upgrade-bundled-skills\default\SKILL.md') -Raw).Trim() -ne 'bundled skill') {
        throw 'New Skill defaults were not retained for comparison.'
    }
    if ((Get-FileHash -LiteralPath (Join-Path $new 'bin\proxy-pool.key')).Hash -ne
        (Get-FileHash -LiteralPath (Join-Path $old 'bin\proxy-pool.key')).Hash) {
        throw 'Proxy encryption key changed during migration.'
    }
    foreach ($path in @((Join-Path $backup 'bin\jwt.key'), (Join-Path $new 'bin\jwt.key'))) {
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne
            (Get-FileHash -LiteralPath (Join-Path $old 'bin\jwt.key') -Algorithm SHA256).Hash) {
            throw 'JWT signing key changed during migration.'
        }
    }

    $rejected = $false
    try {
        & (Join-Path $new 'upgrade-offline.ps1') -OldDirectory $old -BackupDirectory $backup 2>$null
    } catch {
        $rejected = $true
    }
    if (-not $rejected -and $LASTEXITCODE -eq 0) {
        throw 'Reusing a populated migration target was accepted.'
    }
    $otherNew = Join-Path $testRoot 'new-with-key'
    New-Item -ItemType Directory -Path (Join-Path $otherNew 'bin'),
        (Join-Path $otherNew 'runtime\pgsql-18.4\bin') -Force | Out-Null
    '' | Set-Content -LiteralPath (Join-Path $otherNew 'bin\yunkun.exe')
    '' | Set-Content -LiteralPath (Join-Path $otherNew 'runtime\pgsql-18.4\bin\pg_ctl.exe')
    '' | Set-Content -LiteralPath (Join-Path $otherNew 'setup-and-start-offline.ps1')
    'unexpected-release-key' | Set-Content -LiteralPath (Join-Path $otherNew 'bin\jwt.key')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot '..\upgrade-offline.ps1') -Destination $otherNew
    $rejected = $false
    try {
        & (Join-Path $otherNew 'upgrade-offline.ps1') -OldDirectory $old `
            -BackupDirectory (Join-Path $testRoot 'unused-backup') 2>$null
    } catch {
        $rejected = $true
    }
    if (-not $rejected -and $LASTEXITCODE -eq 0) {
        throw 'A release containing a JWT signing key was accepted.'
    }
    if (Test-Path -LiteralPath (Join-Path $testRoot 'unused-backup')) {
        throw 'Rejected migration created a backup directory.'
    }
    Write-Host 'Offline upgrade smoke test passed.'
} finally {
    $resolved = [IO.Path]::GetFullPath($testRoot)
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
    if ($resolved.StartsWith($temp + '\yunkun-upgrade-test-', [StringComparison]::OrdinalIgnoreCase) -and
        (Test-Path -LiteralPath $resolved)) {
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
