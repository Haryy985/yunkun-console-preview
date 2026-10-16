# Run from the freshly extracted release directory after verifying the ZIP SHA-256.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$OldDirectory,
    [Parameter(Mandatory)][string]$BackupDirectory
)

$ErrorActionPreference = 'Stop'

function Resolve-ExistingDirectory([string]$Path) {
    $item = Get-Item -LiteralPath $Path -ErrorAction Stop
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw "Expected a regular directory: $Path"
    }
    return $item.FullName.TrimEnd('\')
}

function Test-Within([string]$Path, [string]$Parent) {
    return $Path.Equals($Parent, [StringComparison]::OrdinalIgnoreCase) -or
        $Path.StartsWith($Parent + '\', [StringComparison]::OrdinalIgnoreCase)
}

function Assert-NoLinks([string]$Path) {
    $links = @(Get-ChildItem -LiteralPath $Path -Force -Recurse -Attributes ReparsePoint -ErrorAction Stop)
    if ($links.Count) { throw "Linked paths are not supported in an offline migration: $($links[0].FullName)" }
}

function Assert-SameFiles([string]$Source, [string]$Destination) {
    $files = @(Get-ChildItem -LiteralPath $Source -Force -File -Recurse)
    foreach ($file in $files) {
        $relative = $file.FullName.Substring($Source.Length).TrimStart('\')
        $copy = Join-Path $Destination $relative
        if (-not (Test-Path -LiteralPath $copy -PathType Leaf) -or
            $file.Length -ne (Get-Item -LiteralPath $copy).Length -or
            (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash -ne
            (Get-FileHash -LiteralPath $copy -Algorithm SHA256).Hash) {
            throw "Backup or migration verification failed: $relative"
        }
    }
}

function Protect-Directory([string]$Path) {
    $acl = Get-Acl -LiteralPath $Path
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($rule in @($acl.Access)) { $acl.RemoveAccessRuleSpecific($rule) }
    foreach ($sid in @(
        [Security.Principal.WindowsIdentity]::GetCurrent().User,
        [Security.Principal.SecurityIdentifier]::new('S-1-5-18')
    )) {
        $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
            $sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
    }
    if ($PSVersionTable.PSEdition -eq 'Core') {
        [IO.FileSystemAclExtensions]::SetAccessControl([IO.DirectoryInfo]::new($Path), $acl)
    } else {
        [IO.DirectoryInfo]::new($Path).SetAccessControl($acl)
    }
}

function Copy-Contents([string]$Source, [string]$Destination) {
    foreach ($item in Get-ChildItem -LiteralPath $Source -Force) {
        Copy-Item -LiteralPath $item.FullName -Destination $Destination -Recurse -Force -ErrorAction Stop
    }
}

try {
    $old = Resolve-ExistingDirectory $OldDirectory
    $new = Resolve-ExistingDirectory $PSScriptRoot
    $backup = [IO.Path]::GetFullPath($BackupDirectory).TrimEnd('\')
    if ((Test-Within $new $old) -or (Test-Within $old $new) -or
        (Test-Within $backup $old) -or (Test-Within $backup $new) -or
        (Test-Within $old $backup) -or (Test-Within $new $backup)) {
        throw 'Old, new and backup directories must be separate and non-nested.'
    }
    if ((Test-Path -LiteralPath $backup) -or (Test-Path -LiteralPath (Join-Path $new '.yunkun-local'))) {
        throw 'Backup destination or new installation data already exists. Nothing was overwritten.'
    }
    $oldJWTKey = Join-Path $old 'bin\jwt.key'
    $newJWTKey = Join-Path $new 'bin\jwt.key'
    if (Test-Path -LiteralPath $newJWTKey) {
        throw 'New release already contains a JWT signing key. Refusing to overwrite private credentials.'
    }
    if ((Test-Path -LiteralPath $oldJWTKey) -and
        (-not (Test-Path -LiteralPath $oldJWTKey -PathType Leaf) -or
         (Get-Item -LiteralPath $oldJWTKey).Length -lt 32)) {
        throw 'Old JWT signing key is invalid. Preserve the old installation for inspection.'
    }
    $parent = Split-Path -Parent $backup
    $null = Resolve-ExistingDirectory $parent
    $private = Join-Path $old '.yunkun-local'
    $stateFile = Join-Path $private 'installation.json'
    foreach ($required in @(
        $stateFile, (Join-Path $private 'config.json'),
        (Join-Path $private 'postgres-data\PG_VERSION'),
        (Join-Path $new 'bin\yunkun.exe'),
        (Join-Path $new 'runtime\pgsql-18.4\bin\pg_ctl.exe'),
        (Join-Path $new 'setup-and-start-offline.ps1')
    )) {
        if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
            throw "Missing required installation file: $required"
        }
    }
    $state = Get-Content -LiteralPath $stateFile -Raw | ConvertFrom-Json
    if ($state.format -ne 1 -or $state.databasePort -lt 1024 -or $state.databasePort -gt 65535 -or
        (Get-Content -LiteralPath (Join-Path $private 'postgres-data\PG_VERSION') -Raw).Trim() -ne '18') {
        throw 'Unsupported installation state or PostgreSQL major version.'
    }
    Assert-NoLinks $old
    Assert-NoLinks $new
    $running = @(Get-CimInstance Win32_Process -Filter "Name = 'yunkun.exe' OR Name = 'postgres.exe'" |
        Where-Object { $_.ExecutablePath -and (Test-Within $_.ExecutablePath $old) })
    if ($running.Count) {
        throw "Old installation still has running processes: $($running.ProcessId -join ', '). Stop it first."
    }
    if (@(Get-NetTCPConnection -State Listen -LocalPort $state.databasePort -ErrorAction SilentlyContinue).Count) {
        throw "Database port $($state.databasePort) is still in use. Confirm the old database stopped."
    }

    Write-Host "Backing up the entire old installation to $backup"
    New-Item -ItemType Directory -Path $backup -ErrorAction Stop | Out-Null
    Protect-Directory $backup
    Copy-Contents $old $backup
    Assert-SameFiles $old $backup
    Write-Host 'Backup verified. Copying private installation data to the new directory.'
    $newPrivate = Join-Path $new '.yunkun-local'
    New-Item -ItemType Directory -Path $newPrivate -ErrorAction Stop | Out-Null
    Protect-Directory $newPrivate
    Copy-Contents $private $newPrivate
    Assert-SameFiles $private $newPrivate
    if (Test-Path -LiteralPath $oldJWTKey -PathType Leaf) {
        Copy-Item -LiteralPath $oldJWTKey -Destination $newJWTKey -ErrorAction Stop
        if ((Get-FileHash -LiteralPath $oldJWTKey -Algorithm SHA256).Hash -ne
            (Get-FileHash -LiteralPath $newJWTKey -Algorithm SHA256).Hash) {
            throw 'JWT signing key migration verification failed.'
        }
    }

    $oldSkills = Join-Path $old 'skills'
    $newSkills = Join-Path $new 'skills'
    if (Test-Path -LiteralPath $oldSkills -PathType Container) {
        New-Item -ItemType Directory -Path $newSkills -Force | Out-Null
        Copy-Contents $oldSkills $newSkills
        Assert-SameFiles $oldSkills $newSkills
    }
    $newConfig = Join-Path $new '.yunkun-local\config.json'
    $config = Get-Content -LiteralPath $newConfig -Raw | ConvertFrom-Json
    if ($config.skill_dir -and [string]::Equals($config.skill_dir, $oldSkills, [StringComparison]::OrdinalIgnoreCase)) {
        $config.skill_dir = $newSkills
        [IO.File]::WriteAllText($newConfig, ($config | ConvertTo-Json -Depth 64),
            [Text.UTF8Encoding]::new($false))
    }
    Write-Host 'Migration complete. Old installation and backup were preserved.'
    Write-Host "Start the new installation: $new\setup-and-start-offline.bat"
    exit 0
} catch {
    Write-Error "Upgrade stopped: $($_.Exception.Message). Keep the old installation and backup. Do not start an incomplete new installation."
    exit 1
}
