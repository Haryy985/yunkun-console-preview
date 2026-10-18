# Yunkun offline Windows launcher.
[CmdletBinding()]
param(
    [switch]$NoBrowser,
    [switch]$Stop,
    [switch]$Public,
    [ValidateRange(1024, 65535)][int]$DatabasePort = 5433,
    [ValidateRange(1024, 65535)][int]$ConsolePort = 5173
)

$ErrorActionPreference = 'Stop'
$script:DatabasePortExplicit = $PSBoundParameters.ContainsKey('DatabasePort')

function Get-SafePath([string]$Root, [string]$Child) {
    $base = [IO.Path]::GetFullPath($Root).TrimEnd('\')
    $full = [IO.Path]::GetFullPath($Child)
    if (-not $full.StartsWith($base + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw "Path escaped the managed directory: $full"
    }
    return $full
}

function Write-Json([string]$Path, $Value) {
    $temp = "$Path.new"
    [IO.File]::WriteAllText($temp, ($Value | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temp -Destination $Path -Force
}

function New-Secret {
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return ([BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
}

function Protect-LocalDirectory([string]$Path) {
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

function Invoke-Native([string]$File, [string[]]$Arguments) {
    if (-not (Test-Path -LiteralPath $File -PathType Leaf)) {
        throw "Required executable not found: $File"
    }
    if ([IO.Path]::GetFileName($File) -ieq 'pg_ctl.exe') {
        $tag = [Guid]::NewGuid().ToString('N')
        $stdout = Join-Path $script:LocalHome "logs\pgctl-$tag.out.log"
        $stderr = Join-Path $script:LocalHome "logs\pgctl-$tag.err.log"
        $quoted = @($Arguments | ForEach-Object {
            if ($_.Contains('"') -or $_.EndsWith('\')) { throw 'Unexpected PostgreSQL control argument.' }
            '"' + $_ + '"'
        })
        $control = Start-Process -FilePath $File -ArgumentList $quoted -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput $stdout -RedirectStandardError $stderr
        try {
            $null = $control.Handle
            if (-not $control.WaitForExit(75000)) {
                throw 'PostgreSQL control command timed out. Check .yunkun-local\logs.'
            }
            $control.Refresh()
            $result = $control.ExitCode
            foreach ($path in @($stdout, $stderr)) {
                if (Test-Path -LiteralPath $path) {
                    Get-Content -LiteralPath $path | ForEach-Object { Write-Host "$_" }
                }
            }
            if ($null -eq $result -or $result -ne 0) {
                throw "pg_ctl.exe failed (exit $result)."
            }
        } finally {
            $control.Dispose()
        }
        return
    }
    $global:LASTEXITCODE = $null
    & $File @Arguments 2>&1 | ForEach-Object { Write-Host "$_" }
    if ($null -eq $global:LASTEXITCODE -or $global:LASTEXITCODE -ne 0) {
        throw "$([IO.Path]::GetFileName($File)) failed (exit $global:LASTEXITCODE)."
    }
}

function Get-PortOwners([int]$Port) {
    @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty OwningProcess -Unique)
}

function Test-OwnedProcess([int]$ProcessId, [string]$Executable) {
    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if (-not $process) { return $false }
    try {
        return [string]::Equals($process.Path, $Executable, [StringComparison]::OrdinalIgnoreCase)
    } catch {
        return $false
    }
}

function Assert-ManagedPostgres([string]$DataPath, [string]$PostgresPath) {
    $pidPath = Join-Path $DataPath 'postmaster.pid'
    if (-not (Test-Path -LiteralPath $pidPath)) {
        throw 'Managed PostgreSQL PID file is missing.'
    }
    $lines = @(Get-Content -LiteralPath $pidPath)
    if ($lines.Count -lt 2) { throw 'Managed PostgreSQL PID file is invalid.' }
    $postgresProcessId = [int]$lines[0]
    if (-not [string]::Equals([IO.Path]::GetFullPath($lines[1]), [IO.Path]::GetFullPath($DataPath), [StringComparison]::OrdinalIgnoreCase) -or
        -not (Test-OwnedProcess $postgresProcessId $PostgresPath)) {
        throw 'Database process ownership does not match this offline installation.'
    }
}

function Invoke-LocalSQL([string]$PsqlPath, $State, [string]$Sql, [switch]$Application) {
    $sqlFile = Join-Path $script:LocalHome 'bootstrap.sql'
    [IO.File]::WriteAllText($sqlFile, $Sql, [Text.UTF8Encoding]::new($false))
    $oldPassword = $env:PGPASSWORD
    try {
        $user = 'yunkun_admin'
        $database = 'postgres'
        $env:PGPASSWORD = $State.adminPassword
        if ($Application) {
            $user = 'yunkun'
            $database = 'yunkun'
            $env:PGPASSWORD = $State.appPassword
        }
        Invoke-Native $PsqlPath @(
            '-X', '-w', '-h', '127.0.0.1', '-p', "$($State.databasePort)",
            '-U', $user, '-d', $database, '-v', 'ON_ERROR_STOP=1', '-f', $sqlFile
        )
    } finally {
        $env:PGPASSWORD = $oldPassword
        Remove-Item -LiteralPath $sqlFile -Force -ErrorAction SilentlyContinue
    }
}

function Test-Ready {
    try {
        $health = Invoke-RestMethod "http://127.0.0.1:$ConsolePort/api/health" -TimeoutSec 3
        $auth = Invoke-RestMethod "http://127.0.0.1:$ConsolePort/api/auth/status" -TimeoutSec 3
        return ($health.ok -eq $true -and $health.ready -eq $true -and
            $health.service -eq 'yunkun' -and $auth.initialized -is [bool])
    } catch {
        return $false
    }
}

function Test-VCRuntime {
    foreach ($name in @('vcruntime140.dll', 'vcruntime140_1.dll', 'msvcp140.dll')) {
        if (-not (Test-Path -LiteralPath (Join-Path $env:WINDIR "System32\$name") -PathType Leaf)) {
            return $false
        }
    }
    return $true
}

function Ensure-VCRuntime([string]$Root) {
    if (Test-VCRuntime) { return }
    $installer = Join-Path $Root 'runtime\vc_redist.x64.exe'
    if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) {
        throw 'Microsoft Visual C++ x64 runtime is missing. The package has no runtime installer.'
    }
    Write-Host 'Microsoft Visual C++ x64 runtime is missing. Installing the bundled offline runtime...' -ForegroundColor Yellow
    try {
        $setup = Start-Process -FilePath $installer -ArgumentList @('/install', '/quiet', '/norestart') `
            -Verb RunAs -Wait -PassThru
        $exitCode = $setup.ExitCode
    } catch {
        throw "Could not install the bundled Microsoft Visual C++ x64 runtime: $($_.Exception.Message)"
    }
    if ($exitCode -notin @(0, 1638, 3010) -or -not (Test-VCRuntime)) {
        throw "Microsoft Visual C++ x64 runtime installation failed (exit $exitCode). Run runtime\vc_redist.x64.exe as Administrator."
    }
}

function Invoke-OfflineSetup {
    if ($env:OS -ne 'Windows_NT' -or -not [Environment]::Is64BitProcess -or
        $env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
        throw 'This offline package requires Windows x64 and 64-bit PowerShell.'
    }
    if ($DatabasePort -eq $ConsolePort) { throw 'DatabasePort must not use the console port.' }

    $root = [IO.Path]::GetFullPath($PSScriptRoot)
    $script:LocalHome = Get-SafePath $root (Join-Path $root '.yunkun-local')
    $runtime = Get-SafePath $root (Join-Path $root 'runtime\pgsql-18.4')
    $app = Get-SafePath $root (Join-Path $root 'bin\yunkun.exe')
    $pgBin = Get-SafePath $runtime (Join-Path $runtime 'bin')
    $pgData = Get-SafePath $script:LocalHome (Join-Path $script:LocalHome 'postgres-data')
    $dataDir = Get-SafePath $script:LocalHome (Join-Path $script:LocalHome 'data')
    $configPath = Get-SafePath $script:LocalHome (Join-Path $script:LocalHome 'config.json')
    $statePath = Get-SafePath $script:LocalHome (Join-Path $script:LocalHome 'installation.json')
    $runPath = Get-SafePath $script:LocalHome (Join-Path $script:LocalHome 'process.json')

    foreach ($required in @(
        (Join-Path $pgBin 'pg_ctl.exe'),
        (Join-Path $pgBin 'initdb.exe'),
        (Join-Path $pgBin 'psql.exe'),
        $app
    )) {
        if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
            throw "Offline runtime file is missing: $required"
        }
    }

    New-Item -ItemType Directory -Path $script:LocalHome -Force | Out-Null
    Protect-LocalDirectory $script:LocalHome
    New-Item -ItemType Directory -Path (Join-Path $script:LocalHome 'logs') -Force | Out-Null

    $state = $null
    if (Test-Path -LiteralPath $statePath) {
        $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
        if ($state.format -ne 1 -or $state.databasePort -lt 1024 -or
            $state.databasePort -gt 65535 -or $state.adminPassword -notmatch '^[a-f0-9]{64}$' -or
            $state.appPassword -notmatch '^[a-f0-9]{64}$') {
            throw 'Invalid offline installation state. Preserve it for diagnostics; do not reset it.'
        }
        if ($script:DatabasePortExplicit -and $DatabasePort -ne $state.databasePort) {
            throw 'DatabasePort cannot change after the offline installation is initialized.'
        }
    }

    $run = $null
    if (Test-Path -LiteralPath $runPath) {
        $run = Get-Content -LiteralPath $runPath -Raw | ConvertFrom-Json
    }
    if ($Stop) {
        if (-not $state) { Write-Host 'No offline installation found.'; return }
        if ((Read-Host 'Stop this Yunkun console and its private database? Type STOP') -cne 'STOP') {
            Write-Host 'Cancelled.'
            return
        }
        if ($run -and (Test-OwnedProcess $run.pid $app)) {
            Stop-Process -Id $run.pid -ErrorAction SilentlyContinue
            Wait-Process -Id $run.pid -Timeout 10 -ErrorAction SilentlyContinue
        }
        if (Test-Path -LiteralPath (Join-Path $pgData 'postmaster.pid')) {
            Assert-ManagedPostgres $pgData (Join-Path $pgBin 'postgres.exe')
            Invoke-Native (Join-Path $pgBin 'pg_ctl.exe') @(
                'stop', '-D', $pgData, '-m', 'fast', '-w', '-t', '60'
            )
        }
        Write-Host 'Offline console stopped. Private data was preserved.'
        return
    }

    Ensure-VCRuntime $root

    $owners = @(Get-PortOwners $ConsolePort)
    if ($run -and (Test-OwnedProcess $run.pid $app)) {
        if ($owners.Count -ne 1 -or $owners[0] -ne $run.pid -or -not (Test-Ready)) {
            throw 'The offline console process exists but is not ready. Check .yunkun-local\logs.'
        }
        Write-Host "Already ready: http://127.0.0.1:$ConsolePort/"
        if (-not $NoBrowser) { Start-Process "http://127.0.0.1:$ConsolePort/" }
        return
    }
    if ($owners.Count) { throw "Port $ConsolePort is occupied (PID $($owners -join ', ')). No process was stopped." }
    if (-not $state) {
        $state = [PSCustomObject]@{
            format = 1
            databasePort = $DatabasePort
            adminPassword = New-Secret
            appPassword = New-Secret
        }
        Write-Json $statePath $state
    }
    if ($state.databasePort -eq $ConsolePort) { throw 'Saved database port conflicts with the console port.' }
    if (-not (Test-Path -LiteralPath $configPath)) {
        Write-Json $configPath @{
            database = @{
                host = '127.0.0.1'
                port = $state.databasePort
                user = 'yunkun'
                dbname = 'yunkun'
                password = $state.appPassword
                sslmode = 'disable'
            }
            skill_dir = (Join-Path $root 'skills')
        }
    }

    if (-not (Test-Path -LiteralPath (Join-Path $pgData 'PG_VERSION'))) {
        if ((Test-Path -LiteralPath $pgData) -and @(Get-ChildItem -LiteralPath $pgData -Force).Count) {
            throw 'Incomplete private database directory. Automatic reinitialization is forbidden.'
        }
        $pwFile = Join-Path $script:LocalHome 'initdb-password.tmp'
        try {
            [IO.File]::WriteAllText($pwFile, $state.adminPassword + "`n", [Text.UTF8Encoding]::new($false))
            Invoke-Native (Join-Path $pgBin 'initdb.exe') @(
                '-D', $pgData, '-U', 'yunkun_admin', '--encoding=UTF8',
                '--locale=C', '--auth=scram-sha-256', "--pwfile=$pwFile"
            )
        } finally {
            Remove-Item -LiteralPath $pwFile -Force -ErrorAction SilentlyContinue
        }
    }
    if ([IO.File]::ReadAllText((Join-Path $pgData 'PG_VERSION')).Trim() -ne '18') {
        throw 'Unexpected PostgreSQL data major version. Automatic upgrade is forbidden.'
    }

    $dbOwners = @(Get-PortOwners $state.databasePort)
    if ($dbOwners.Count) {
        Assert-ManagedPostgres $pgData (Join-Path $pgBin 'postgres.exe')
    } else {
        Invoke-Native (Join-Path $pgBin 'pg_ctl.exe') @(
            'start', '-D', $pgData, '-l', (Join-Path $script:LocalHome 'logs\postgres.log'),
            '-o', "-h 127.0.0.1 -p $($state.databasePort)", '-w', '-t', '60'
        )
    }

    $env:PGCONNECT_TIMEOUT = '5'
    $env:PGSSLMODE = 'disable'
    $env:YUNKUN_SETUP_ROLE_PASSWORD = $state.appPassword
    try {
        Invoke-LocalSQL (Join-Path $pgBin 'psql.exe') $state @'
\getenv rolepass YUNKUN_SETUP_ROLE_PASSWORD
SELECT format('CREATE ROLE yunkun LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD %L', :'rolepass')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'yunkun') \gexec
SELECT 'CREATE DATABASE yunkun OWNER yunkun'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'yunkun') \gexec
'@
        Invoke-LocalSQL (Join-Path $pgBin 'psql.exe') $state 'SELECT 1 AS database_ready;' -Application
    } finally {
        $env:YUNKUN_SETUP_ROLE_PASSWORD = $null
    }

    New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
    $env:YUNKUN_CONFIG = $configPath
    $env:YUNKUN_PG_DSN = $null
    $env:YUNKUN_SKILL_DIR = Join-Path $root 'skills'
    $listenAddress = if ($Public) { '0.0.0.0' } else { '127.0.0.1' }
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $process = Start-Process -FilePath $app -WorkingDirectory $script:LocalHome -WindowStyle Hidden -PassThru `
        -ArgumentList @('-addr', "$listenAddress`:$ConsolePort", '-proxy=', '-data', ('"' + $dataDir + '"')) `
        -RedirectStandardOutput (Join-Path $script:LocalHome "logs\console-$stamp.out.log") `
        -RedirectStandardError (Join-Path $script:LocalHome "logs\console-$stamp.err.log")
    Write-Json $runPath @{ pid = $process.Id; started = $process.StartTime.ToUniversalTime().ToString('o') }

    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        if ($process.HasExited) { throw 'Offline console exited before readiness. Check .yunkun-local\logs.' }
        if (@(Get-PortOwners $ConsolePort).Count -eq 1 -and (Test-Ready)) {
            Write-Host "Ready: http://127.0.0.1:$ConsolePort/"
            Write-Host "Console listen address: $listenAddress`:$ConsolePort. Private PostgreSQL: $($state.databasePort)."
            Write-Host 'Offline mode: no dependency download or compilation was performed.'
            Write-Host 'First login: username yunkun; set your own console password in the browser.'
            if (-not $NoBrowser) { Start-Process "http://127.0.0.1:$ConsolePort/" }
            return
        }
        Start-Sleep -Seconds 2
    }
    throw 'Offline console is not ready. Check .yunkun-local\logs.'
}

try {
    Invoke-OfflineSetup
    exit 0
} catch {
    Write-Host "`nOffline setup/start failed: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host 'No network access, firewall change, system service installation, or automatic database reset was performed.'
    exit 1
}
