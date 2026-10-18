# Requires Windows PowerShell 5.1. Dot-source to load helpers without starting services.
[CmdletBinding()]
param(
    [switch]$Rebuild,
    [switch]$NoBrowser,
    [switch]$Stop,
    [ValidateRange(1024, 65535)][int]$DatabasePort = 5433
)
$script:DatabasePortExplicit = $PSBoundParameters.ContainsKey('DatabasePort')

function Assert-LocalPath([string]$Root, [string]$Path) {
    $base = [IO.Path]::GetFullPath($Root).TrimEnd('\')
    $full = [IO.Path]::GetFullPath($Path)
    if (-not $full.StartsWith($base + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw "Path escaped the managed directory: $Path"
    }
    $cursor = $full
    while ($cursor -and $cursor.Length -ge $base.Length) {
        if (Test-Path -LiteralPath $cursor) {
            $item = Get-Item -LiteralPath $cursor -Force
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                throw "Linked directories are not allowed in managed paths: $cursor"
            }
        }
        $cursor = Split-Path -Parent $cursor
    }
    return $full
}

function Write-LocalJson([string]$Path, $Value) {
    $temp = "$Path.new"
    [IO.File]::WriteAllText($temp, ($Value | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temp -Destination $Path -Force
}

function New-LocalPassword {
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return ([BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
}

function Protect-LocalDirectory([string]$Path) {
    $acl = Get-Acl -LiteralPath $Path
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($rule in @($acl.Access)) { $acl.RemoveAccessRuleSpecific($rule) }
    foreach ($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User,
        [Security.Principal.SecurityIdentifier]::new('S-1-5-18'))) {
        $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
            $sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
    }
    # Persist changed access rules only; Set-Acl can also request audit privileges.
    if ($PSVersionTable.PSEdition -eq 'Core') {
        [IO.FileSystemAclExtensions]::SetAccessControl([IO.DirectoryInfo]::new($Path), $acl)
    } else {
        [IO.DirectoryInfo]::new($Path).SetAccessControl($acl)
    }
}

function Test-ArchiveHash([string]$Path, [string]$Expected) {
    if ($Expected -notmatch '^[a-fA-F0-9]{64}$') { throw 'Invalid dependency SHA256.' }
    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $Expected) {
        throw "Download SHA256 mismatch: $([IO.Path]::GetFileName($Path)). No downloaded code was executed."
    }
}

function Expand-CheckedArchive([string]$Archive, [string]$Destination, [string[]]$Includes = @()) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Includes = @($Includes | Where-Object { $_ })
    $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        foreach ($entry in $zip.Entries) {
            $name = $entry.FullName.Replace('/', '\')
            if ($name -match '(^[\\/]|:|(^|[\\/])\.\.([\\/]|$))' -or
                (($entry.ExternalAttributes -shr 16) -band 0xF000) -eq 0xA000) {
                throw "Unsafe archive member: $($entry.FullName)"
            }
            $null = Assert-LocalPath $Destination (Join-Path $Destination $name)
        }
        $selected = @($zip.Entries | Where-Object {
            $member = $_.FullName.Replace('\', '/')
            $allowed = $Includes.Count -eq 0
            foreach ($prefix in $Includes) {
                if ($member.StartsWith($prefix, [StringComparison]::Ordinal)) { $allowed = $true }
            }
            $allowed
        })
        foreach ($entry in $selected) {
            $target = Join-Path $Destination $entry.FullName.Replace('/', '\')
            if ($target.Length -gt 245) {
                throw 'Installation path is too long for Windows PowerShell 5.1. Extract the source to a shorter local directory.'
            }
        }
        foreach ($entry in $selected) {
            $target = Join-Path $Destination $entry.FullName.Replace('/', '\')
            if ($entry.FullName.EndsWith('/') -or $entry.FullName.EndsWith('\')) {
                $null = [IO.Directory]::CreateDirectory($target)
            } else {
                $null = [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
                [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $target, $false)
            }
        }
    } finally { $zip.Dispose() }
}

function Invoke-LocalNative([string]$File, [string[]]$Arguments) {
    if (-not (Test-Path -LiteralPath $File -PathType Leaf)) { throw "Required executable not found: $File" }
    if ([IO.Path]::GetFileName($File) -ieq 'pg_ctl.exe') {
        # A running postgres child can retain pipe handles after pg_ctl exits.
        $tag = [Guid]::NewGuid().ToString('N')
        $output = Join-Path $script:LocalHome "logs\pgctl-$tag.out.log"
        $errors = Join-Path $script:LocalHome "logs\pgctl-$tag.err.log"
        $quoted = @($Arguments | ForEach-Object {
            if ($_.Contains('"') -or $_.EndsWith('\')) { throw 'Unexpected database control argument.' }
            '"' + $_ + '"'
        })
        $control = Start-Process -FilePath $File -ArgumentList $quoted -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput $output -RedirectStandardError $errors
        try {
            # Retain the handle before waiting so Windows PowerShell preserves ExitCode.
            $null = $control.Handle
            if (-not $control.WaitForExit(75000)) { throw 'Database control timed out. Check managed logs before retrying.' }
            $control.Refresh()
            $result = $control.ExitCode
            foreach ($path in @($output, $errors)) {
                if (Test-Path -LiteralPath $path) { Get-Content -LiteralPath $path | ForEach-Object { Write-Host $_ } }
            }
            if ($null -eq $result -or $result -ne 0) { throw "pg_ctl.exe failed (exit $result)." }
        } finally { $control.Dispose() }
        return
    }
    # Native tools routinely write progress to stderr, even with exit code zero.
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $global:LASTEXITCODE = $null
        & $File @Arguments 2>&1 | ForEach-Object { Write-Host "$_" }
        $result = $global:LASTEXITCODE
    } finally { $ErrorActionPreference = $previous }
    if ($null -eq $result -or $result -ne 0) { throw "$([IO.Path]::GetFileName($File)) failed (exit $result)." }
}

function Get-LocalDependency($Spec, [string]$Tools, [string]$Cache) {
    $destination = Assert-LocalPath $Tools (Join-Path $Tools "$($Spec.directory)-$($Spec.version)")
    $binary = Join-Path $destination $Spec.executable
    $receipt = Join-Path $destination '.verified-sha256'
    if ((Test-Path -LiteralPath $binary) -and (Test-Path -LiteralPath $receipt) -and
        ([IO.File]::ReadAllText($receipt).Trim() -eq $Spec.sha256)) { return $binary }
    if (Test-Path -LiteralPath $destination) {
        throw "Incomplete or unverified tool directory: $destination. Preserve it outside the tools directory, then retry."
    }
    $uri = [Uri]$Spec.url
    if ($uri.Scheme -ne 'https' -or $uri.Host -notin @('go.dev', 'nodejs.org', 'get.enterprisedb.com')) {
        throw 'Dependency URL is not an approved HTTPS source.'
    }
    $archive = Assert-LocalPath $Cache (Join-Path $Cache ([IO.Path]::GetFileName($uri.LocalPath)))
    if (-not (Test-Path -LiteralPath $archive)) {
        Write-Host "Downloading $($Spec.directory) $($Spec.version)..."
        $partial = "$archive.partial"
        $progress = $ProgressPreference
        try {
            $ProgressPreference = 'SilentlyContinue'
            Invoke-WebRequest -UseBasicParsing -Uri $uri -OutFile $partial -TimeoutSec 1800
            Test-ArchiveHash $partial $Spec.sha256
            Move-Item -LiteralPath $partial -Destination $archive -Force
        } finally { $ProgressPreference = $progress }
    }
    Test-ArchiveHash $archive $Spec.sha256
    $stage = Assert-LocalPath $Tools (Join-Path $Tools ("unpack-" + [Guid]::NewGuid().ToString('N')))
    New-Item -ItemType Directory -Path $stage | Out-Null
    try {
        Write-Host "Extracting verified $($Spec.directory) $($Spec.version)..."
        Expand-CheckedArchive $archive $stage @($Spec.includePrefixes)
        $unpacked = Assert-LocalPath $stage (Join-Path $stage $Spec.directory)
        if (-not (Test-Path -LiteralPath (Join-Path $unpacked $Spec.executable))) {
            throw "Archive is missing the expected executable: $($Spec.executable)"
        }
        Move-Item -LiteralPath $unpacked -Destination $destination
        [IO.File]::WriteAllText($receipt, $Spec.sha256)
    } finally {
        $null = Assert-LocalPath $Tools $stage
        if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
    }
    return $binary
}

function Get-SourceFingerprint([string]$Root) {
    $files = @(
        Get-ChildItem -LiteralPath $Root -File |
            Where-Object { $_.Name -in @('go.mod', 'go.sum', 'VERSION', 'setup-and-start.ps1', 'setup-dependencies.json') }
        foreach ($directory in @('agent', 'cmd', 'config', 'db', 'enrich', 'evidence', 'guard',
            'intercept', 'llmpool', 'llmrec', 'mcphttp', 'notify', 'proxypool', 'rag',
            'report', 'selfupdate', 'server', 'sidequestion', 'traffic', 'web')) {
            # Enumerate one level at a time so node_modules is never traversed.
            $pending = New-Object 'Collections.Generic.Stack[string]'
            $pending.Push((Join-Path $Root $directory))
            while ($pending.Count -gt 0) {
                foreach ($item in Get-ChildItem -LiteralPath $pending.Pop() -Force) {
                    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                        throw "Source links are not supported: $($item.FullName)"
                    }
                    if ($item.PSIsContainer) {
                        $generatedUI = $item.Parent.FullName -eq (Join-Path $Root 'server\webui') -and
                            $item.Name -in @('current', 'release103')
                        if ($item.Name -notin @('node_modules', '.next', 'out', 'dist', '.git') -and -not $generatedUI) {
                            $pending.Push($item.FullName)
                        }
                    } elseif ($item.Name -notin @('tsconfig.tsbuildinfo', 'next-env.d.ts') -and
                        $item.Name -notlike '.env*') { $item }
                }
            }
        }
    )
    $lines = foreach ($file in $files | Sort-Object FullName) {
        $relative = $file.FullName.Substring($Root.Length + 1)
        "$relative $((Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash)"
    }
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        return ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes(($lines -join "`n"))))).Replace('-', '')
    } finally { $sha.Dispose() }
}

function Get-PortOwners([int]$Port) {
    return @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty OwningProcess -Unique)
}

function Test-OwnedProcess([int]$ProcessId, [string]$Executable, [string]$Started = '') {
    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if (-not $process) { return $false }
    if (-not [string]::Equals($process.Path, $Executable, [StringComparison]::OrdinalIgnoreCase)) { return $false }
    return (-not $Started -or $process.StartTime.ToUniversalTime().ToString('o') -eq $Started)
}

function Assert-ClusterProcess([string]$PgData, [string]$PgBin, [int[]]$Owners = @()) {
    $pidFile = Join-Path $PgData 'postmaster.pid'
    if (-not (Test-Path -LiteralPath $pidFile)) { throw 'Managed database PID file is missing.' }
    $lines = @(Get-Content -LiteralPath $pidFile)
    if ($lines.Count -lt 4 -or
        -not [string]::Equals([IO.Path]::GetFullPath($lines[1]), $PgData, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Test-OwnedProcess ([int]$lines[0]) (Join-Path $PgBin 'postgres.exe')) -or
        ($Owners.Count -and ($Owners.Count -ne 1 -or $Owners[0] -ne [int]$lines[0]))) {
        throw 'Database process ownership does not match the managed cluster. No database was stopped.'
    }
}

function Test-LocalReady {
    try {
        $health = Invoke-RestMethod 'http://127.0.0.1:5173/api/health' -TimeoutSec 3
        $auth = Invoke-RestMethod 'http://127.0.0.1:5173/api/auth/status' -TimeoutSec 3
        return ($health.ok -eq $true -and $health.ready -eq $true -and
            $health.service -eq 'yunkun' -and $auth.initialized -is [bool])
    } catch { return $false }
}

function Invoke-LocalSQL([string]$PgBin, $State, [string]$SQL, [switch]$Application) {
    $sqlFile = Join-Path $script:LocalHome 'bootstrap.sql'
    [IO.File]::WriteAllText($sqlFile, $SQL, [Text.UTF8Encoding]::new($false))
    $previous = $env:PGPASSWORD
    try {
        $user = 'yunkun_admin'; $database = 'postgres'; $env:PGPASSWORD = $State.adminPassword
        if ($Application) {
            $user = 'yunkun'; $database = 'yunkun'; $env:PGPASSWORD = $State.appPassword
        }
        Invoke-LocalNative (Join-Path $PgBin 'psql.exe') @('-X', '-w', '-h', '127.0.0.1',
            '-p', "$($State.databasePort)", '-U', $user, '-d', $database,
            '-v', 'ON_ERROR_STOP=1', '-f', $sqlFile)
    } finally {
        $env:PGPASSWORD = $previous
        Remove-Item -LiteralPath $sqlFile -Force -ErrorAction SilentlyContinue
    }
}

function Assert-ManagedConfig($Config, $State) {
    $db = $Config.database
    if ($db.host -ne '127.0.0.1' -or $db.port -ne $State.databasePort -or
        $db.user -ne 'yunkun' -or $db.dbname -ne 'yunkun' -or
        $db.password -cne $State.appPassword -or $db.sslmode -ne 'disable' -or $db.dsn) {
        throw 'Managed config was modified. Refusing to overwrite it or connect to another database. See ONE_CLICK_WINDOWS.md.'
    }
}

function Invoke-YunkunSetup {
    $ErrorActionPreference = 'Stop'
    if ($env:OS -ne 'Windows_NT' -or -not [Environment]::Is64BitProcess -or
        $env:PROCESSOR_ARCHITECTURE -ne 'AMD64') { throw 'This launcher requires Windows x64 and 64-bit PowerShell.' }
    if ($DatabasePort -eq 5173) { throw 'DatabasePort must not use the console port 5173.' }
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $root = [IO.Path]::GetFullPath($PSScriptRoot)
    $script:LocalHome = Assert-LocalPath $root (Join-Path $root '.yunkun-local')
    New-Item -ItemType Directory -Path $script:LocalHome -Force | Out-Null
    # Protect credentials before creating any files, without changing system policy.
    Protect-LocalDirectory $script:LocalHome
    $lock = $null
    $saved = @{}
    foreach ($key in @('PATH', 'HUSKY', 'GOTOOLCHAIN', 'GOWORK', 'YUNKUN_CONFIG', 'YUNKUN_PG_DSN',
        'YUNKUN_SKILL_DIR', 'PGCONNECT_TIMEOUT', 'PGSSLMODE', 'YUNKUN_SETUP_ROLE_PASSWORD')) {
        $saved[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
    }
    try {
        try { $lock = [IO.File]::Open((Join-Path $script:LocalHome 'launcher.lock'), 'OpenOrCreate', 'ReadWrite', 'None') }
        catch { throw 'Another setup/start/stop operation is running. Wait for it to finish.' }
        $statePath = Join-Path $script:LocalHome 'installation.json'
        $configPath = Join-Path $script:LocalHome 'config.json'
        $state = $null
        if (Test-Path -LiteralPath $statePath) {
            $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
            if ($state.format -ne 1 -or $state.databasePort -lt 1024 -or $state.databasePort -gt 65535 -or
                $state.adminPassword -notmatch '^[a-f0-9]{64}$' -or $state.appPassword -notmatch '^[a-f0-9]{64}$') {
                throw 'Invalid managed installation state. Restore from backup; do not reset the database.'
            }
            if ($script:DatabasePortExplicit -and $DatabasePort -ne $state.databasePort) {
                throw 'DatabasePort cannot change an existing installation. Preserve the saved port.'
            }
        }
        if ($Stop -and -not $state) { Write-Host 'No managed installation found.'; return }
        $app = Join-Path $script:LocalHome 'yunkun.exe'
        $runPath = Join-Path $script:LocalHome 'process.json'
        $run = $null
        if (Test-Path -LiteralPath $runPath) { $run = Get-Content -LiteralPath $runPath -Raw | ConvertFrom-Json }
        $owners = @(Get-PortOwners 5173)
        $ownApp = ($run -and (Test-OwnedProcess $run.pid $app $run.started))
        if ($Stop) {
            $specs = Get-Content -LiteralPath (Join-Path $root 'setup-dependencies.json') -Raw | ConvertFrom-Json
            $pgBin = Join-Path $script:LocalHome "tools\$($specs.postgres.directory)-$($specs.postgres.version)\bin"
            $pgData = Assert-LocalPath $script:LocalHome (Join-Path $script:LocalHome 'postgres-data')
            $dbRunning = Test-Path -LiteralPath (Join-Path $pgData 'postmaster.pid')
            if ($dbRunning) { Assert-ClusterProcess $pgData $pgBin }
            if ((Read-Host 'Stop this managed console and database? Running tasks will be interrupted. Type STOP') -cne 'STOP') {
                Write-Host 'Cancelled.'; return
            }
            if ($ownApp) {
                Stop-Process -Id $run.pid -ErrorAction Stop
                Wait-Process -Id $run.pid -ErrorAction SilentlyContinue
            }
            if ($dbRunning) {
                Invoke-LocalNative (Join-Path $pgBin 'pg_ctl.exe') @('stop', '-D', $pgData, '-m', 'fast', '-w', '-t', '60')
            }
            Write-Host 'Managed services stopped. Data and credentials were preserved.'
            return
        }
        if ($ownApp) {
            if ($Rebuild) { throw 'Stop the managed console before rebuilding. Use stop-local.bat first.' }
            if ($owners.Count -ne 1 -or $owners[0] -ne $run.pid -or -not (Test-LocalReady)) {
                throw 'The managed console is running but not ready. Check .yunkun-local logs; it was not restarted.'
            }
            Write-Host 'Already ready: http://127.0.0.1:5173/'
            if (-not $NoBrowser) { Start-Process 'http://127.0.0.1:5173/' }
            return
        }
        if ($owners.Count) { throw "Port 5173 is occupied (PID $($owners -join ', ')). No process was stopped." }
        if ($env:YUNKUN_PG_DSN -or $env:YUNKUN_CONFIG -or (Test-Path -LiteralPath (Join-Path $root 'config.json'))) {
            throw 'Existing config.json or database environment detected. This new-install launcher will not replace it. Use NATIVE_DEPLOYMENT.md or extract a clean source copy.'
        }
        if (-not $state -and @(Get-PortOwners $DatabasePort).Count) {
            throw "Database port $DatabasePort is occupied. No database was changed. Select a free port explicitly with -DatabasePort."
        }
        foreach ($name in @('tools', 'downloads', 'logs', 'data')) {
            New-Item -ItemType Directory -Path (Assert-LocalPath $script:LocalHome (Join-Path $script:LocalHome $name)) -Force | Out-Null
        }
        $specs = Get-Content -LiteralPath (Join-Path $root 'setup-dependencies.json') -Raw | ConvertFrom-Json
        $tools = Join-Path $script:LocalHome 'tools'; $cache = Join-Path $script:LocalHome 'downloads'
        $pgCtl = Get-LocalDependency $specs.postgres $tools $cache
        $pgBin = Split-Path -Parent $pgCtl
        Invoke-LocalNative (Join-Path $pgBin 'postgres.exe') @('--version')
        if (-not $state) {
            $state = [PSCustomObject]@{ format = 1; databasePort = $DatabasePort
                adminPassword = New-LocalPassword; appPassword = New-LocalPassword }
            Write-LocalJson $statePath $state
        }
        if (-not (Test-Path -LiteralPath $configPath)) {
            Write-LocalJson $configPath @{ database = @{host = '127.0.0.1'; port = $state.databasePort
                user = 'yunkun'; dbname = 'yunkun'; password = $state.appPassword; sslmode = 'disable'}
                skill_dir = (Join-Path $root 'skills') }
        }
        Assert-ManagedConfig (Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json) $state
        $pgData = Assert-LocalPath $script:LocalHome (Join-Path $script:LocalHome 'postgres-data')
        if (-not (Test-Path -LiteralPath (Join-Path $pgData 'PG_VERSION'))) {
            if ((Test-Path -LiteralPath $pgData) -and @(Get-ChildItem -LiteralPath $pgData -Force).Count) {
                throw 'Incomplete database directory. Preserve it for diagnostics; automatic reinitialization is forbidden.'
            }
            $pwFile = Join-Path $script:LocalHome 'initdb-password.tmp'
            try {
                [IO.File]::WriteAllText($pwFile, $state.adminPassword + "`n", [Text.UTF8Encoding]::new($false))
                Invoke-LocalNative (Join-Path $pgBin 'initdb.exe') @('-D', $pgData, '-U', 'yunkun_admin',
                    '--encoding=UTF8', '--locale=C', '--auth=scram-sha-256', "--pwfile=$pwFile")
            } finally { Remove-Item -LiteralPath $pwFile -Force -ErrorAction SilentlyContinue }
        }
        if ([IO.File]::ReadAllText((Join-Path $pgData 'PG_VERSION')).Trim() -ne '18') {
            throw 'Unexpected PostgreSQL data major version. Automatic upgrade is forbidden.'
        }
        $dbOwners = @(Get-PortOwners $state.databasePort)
        if ($dbOwners.Count) {
            Assert-ClusterProcess $pgData $pgBin $dbOwners
        } else {
            Invoke-LocalNative $pgCtl @('start', '-D', $pgData, '-l', (Join-Path $script:LocalHome 'logs\postgres.log'),
                '-o', "-h 127.0.0.1 -p $($state.databasePort)", '-w', '-t', '60')
        }
        $env:PGCONNECT_TIMEOUT = '5'; $env:PGSSLMODE = 'disable'
        $env:YUNKUN_SETUP_ROLE_PASSWORD = $state.appPassword
        Invoke-LocalSQL $pgBin $state @'
\getenv rolepass YUNKUN_SETUP_ROLE_PASSWORD
SELECT format('CREATE ROLE yunkun LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD %L', :'rolepass')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'yunkun') \gexec
SELECT 'CREATE DATABASE yunkun OWNER yunkun'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'yunkun') \gexec
'@
        Invoke-LocalSQL $pgBin $state 'SELECT 1 AS database_ready;' -Application
        $env:YUNKUN_SETUP_ROLE_PASSWORD = $null
        $fingerprint = Get-SourceFingerprint $root
        $buildPath = Join-Path $script:LocalHome 'build.json'
        $build = $null
        if (Test-Path -LiteralPath $buildPath) { $build = Get-Content -LiteralPath $buildPath -Raw | ConvertFrom-Json }
        if ($Rebuild -or -not (Test-Path -LiteralPath $app) -or -not $build -or $build.source -ne $fingerprint -or
            $build.binary -ne (Get-FileHash -LiteralPath $app -Algorithm SHA256).Hash) {
            $go = Get-LocalDependency $specs.go $tools $cache
            $node = Get-LocalDependency $specs.node $tools $cache
            $env:PATH = "$(Split-Path -Parent $node);$(Split-Path -Parent $go);$env:PATH"
            $env:HUSKY = '0'; $env:GOTOOLCHAIN = 'local'; $env:GOWORK = 'off'
            Push-Location (Join-Path $root 'web')
            try {
                Invoke-LocalNative (Join-Path (Split-Path -Parent $node) 'npm.cmd') @('ci', '--no-audit', '--no-fund')
                Invoke-LocalNative (Join-Path (Split-Path -Parent $node) 'npm.cmd') @('run', 'build:static')
            } finally { Pop-Location }
            $assets = Assert-LocalPath $root (Join-Path $root 'server\webui\current')
            $export = Join-Path $root 'web\out'
            if (-not (Test-Path -LiteralPath (Join-Path $export 'index.html'))) { throw 'Static export has no index.html.' }
            if (Test-Path -LiteralPath $assets) { Remove-Item -LiteralPath $assets -Recurse -Force }
            New-Item -ItemType Directory -Path $assets -Force | Out-Null
            Get-ChildItem -LiteralPath $export -Force | ForEach-Object {
                Copy-Item -LiteralPath $_.FullName -Destination $assets -Recurse -Force
            }
            $candidate = Join-Path $script:LocalHome 'yunkun.build.exe'
            $buildVersion = 'dev'
            $versionPath = Join-Path $root 'VERSION'
            if (Test-Path -LiteralPath $versionPath) {
                $buildVersion = [IO.File]::ReadAllText($versionPath).Trim()
                if ($buildVersion -notmatch '^v?\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?$') {
                    throw 'VERSION does not contain a valid release version.'
                }
            }
            Push-Location $root
            try { Invoke-LocalNative $go @('build', '-tags', 'embedui', '-trimpath', '-ldflags', "-X main.version=$buildVersion", '-o', $candidate, './cmd/yunkun') }
            finally { Pop-Location }
            if ((Get-SourceFingerprint $root) -ne $fingerprint) { throw 'Source changed during compilation. Retry after edits finish.' }
            Move-Item -LiteralPath $candidate -Destination $app -Force
            Write-LocalJson $buildPath @{ source = $fingerprint; binary = (Get-FileHash -LiteralPath $app -Algorithm SHA256).Hash }
        } else { Write-Host 'Source unchanged; reusing the verified local build.' }
        $env:YUNKUN_CONFIG = $configPath
        $env:YUNKUN_PG_DSN = $null
        $env:YUNKUN_SKILL_DIR = Join-Path $root 'skills'
        if (@(Get-PortOwners 5173).Count) { throw 'Port 5173 became occupied during setup. No process was stopped.' }
        $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
        $process = Start-Process -FilePath $app -WorkingDirectory $script:LocalHome -WindowStyle Hidden -PassThru `
            -ArgumentList @('-addr', '127.0.0.1:5173', '-proxy=', '-data', 'data') `
            -RedirectStandardOutput (Join-Path $script:LocalHome "logs\console-$stamp.out.log") `
            -RedirectStandardError (Join-Path $script:LocalHome "logs\console-$stamp.err.log")
        Write-LocalJson $runPath @{ pid = $process.Id; started = $process.StartTime.ToUniversalTime().ToString('o') }
        $ready = $false
        for ($attempt = 0; $attempt -lt 60; $attempt++) {
            if ($process.HasExited) { throw 'Console exited before readiness. Check .yunkun-local\logs.' }
            $currentOwners = @(Get-PortOwners 5173)
            if ($currentOwners.Count -eq 1 -and $currentOwners[0] -eq $process.Id -and (Test-LocalReady)) {
                $ready = $true; break
            }
            Start-Sleep -Seconds 2
        }
        if (-not $ready) { throw 'Console is still not ready. Check .yunkun-local\logs; process startup is not successful recovery.' }
        Write-Host 'Ready: http://127.0.0.1:5173/'
        Write-Host "Console port: 5173. Managed PostgreSQL: $($state.databasePort). Capture proxy: disabled."
        Write-Host 'First login: username yunkun; set your own console password in the browser.'
        Write-Host 'Services continue in the background. Use stop-local.bat to stop this installation.'
        if (-not $NoBrowser) { Start-Process 'http://127.0.0.1:5173/' }
    } finally {
        foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key, $saved[$key], 'Process') }
        if ($lock) { $lock.Dispose() }
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    try { Invoke-YunkunSetup; exit 0 }
    catch {
        Write-Host "`nYunkun setup/start failed: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host 'No automatic database reset, system service installation, or firewall changes were performed.'
        exit 1
    }
}
