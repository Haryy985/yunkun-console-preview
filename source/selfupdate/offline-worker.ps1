# Trusted update supervisor. Invoked from the current binary after ZIP validation.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$JobFile)
$ErrorActionPreference = 'Stop'
# A PowerShell 7 parent may pass incompatible built-in module paths to Windows
# PowerShell 5.1. Use only the system modules in this helper process.
$env:PSModulePath = (Join-Path $PSHOME 'Modules') + ';' + (Join-Path $env:ProgramFiles 'WindowsPowerShell\Modules')
$job = Get-Content -LiteralPath $JobFile -Raw | ConvertFrom-Json
$root = [IO.Path]::GetFullPath($job.root).TrimEnd('\')
$work = [IO.Path]::GetFullPath($job.work).TrimEnd('\')
$private = Join-Path $root '.yunkun-local'
$updates = Join-Path $private 'updates'
$candidate = Join-Path $work 'candidate'
$backup = Join-Path $work 'backup'
$stateFile = Join-Path $private 'update-state.json'
$app = Join-Path $root 'bin\yunkun.exe'
$pgData = Join-Path $private 'postgres-data'
$pgBin = Join-Path $root 'runtime\pgsql-18.4\bin'
$lock = $null
$stopped = $false
$backedUp = $false
$changed = $false

function Assert-Within([string]$Path, [string]$Parent) {
    $p = [IO.Path]::GetFullPath($Path)
    $b = [IO.Path]::GetFullPath($Parent).TrimEnd('\') + '\'
    if (-not $p.StartsWith($b, [StringComparison]::OrdinalIgnoreCase)) { throw 'Update path escaped its installation.' }
}

function Set-State([string]$Phase, [string]$Message, [string]$Failure = '') {
    $value = @{ phase=$Phase; percent=-1; message=$Message; error=$Failure; version=$job.to;
        job=$work; backup=$backup; updated_at=[DateTime]::UtcNow.ToString('o') }
    $temp = "$stateFile.new"
    [IO.File]::WriteAllText($temp, ($value | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temp -Destination $stateFile -Force
    Write-Host "$Phase : $Message $Failure"
}

function Invoke-Files([string]$Operation) {
    $result = & (Join-Path $work 'file-helper.exe') -offline-files $JobFile -file-operation $Operation
    if ($LASTEXITCODE -ne 0) { throw "Verified file operation failed: $Operation" }
    return $result
}

function Stop-App {
    $runFile = Join-Path $private 'process.json'
    if (-not (Test-Path -LiteralPath $runFile)) { return }
    $run = Get-Content -LiteralPath $runFile -Raw | ConvertFrom-Json
    $p = Get-Process -Id $run.pid -ErrorAction SilentlyContinue
    if ($p) {
        if ($p.Path -ne $app) { throw 'Refusing to stop an unrelated application.' }
        Stop-Process -Id $p.Id
        if (-not $p.WaitForExit(20000)) { throw 'Application did not stop.' }
        $p.Dispose()
    }
}

function Stop-Database {
    $pidFile = Join-Path $pgData 'postmaster.pid'
    if (-not (Test-Path -LiteralPath $pidFile)) { return }
    $lines = @(Get-Content -LiteralPath $pidFile)
    if ($lines.Count -lt 2 -or [IO.Path]::GetFullPath($lines[1]) -ne $pgData) { throw 'Database ownership mismatch.' }
    $p = Get-Process -Id ([int]$lines[0]) -ErrorAction SilentlyContinue
    if (-not $p -or $p.Path -ne (Join-Path $pgBin 'postgres.exe')) { throw 'Database process ownership mismatch.' }
    $control = Start-Process -FilePath (Join-Path $pgBin 'pg_ctl.exe') -WindowStyle Hidden -PassThru `
        -ArgumentList @('stop', '-D', ('"' + $pgData + '"'), '-m', 'fast', '-w', '-t', '60') `
        -RedirectStandardOutput (Join-Path $work 'pg-stop.out') -RedirectStandardError (Join-Path $work 'pg-stop.err')
    $null = $control.Handle
    if (-not $control.WaitForExit(75000)) { throw 'Private database shutdown timed out.' }
    $control.Refresh()
    if ($control.ExitCode -ne 0) { throw 'Private database shutdown failed; backup was not started.' }
}

function Start-Installation([bool]$Probe) {
    $env:YUNKUN_UPDATE_JOB = $work
    $env:YUNKUN_UPDATE_PROBE = if ($Probe) { '1' } else { '' }
    $ps = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $args = @('-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File',
        ('"' + (Join-Path $root 'setup-and-start-offline.ps1') + '"'),
        '-NoBrowser', '-ConsolePort', "$($job.console_port)", '-DatabasePort', "$($job.database_port)")
    if ($job.public) { $args += '-Public' }
    $tag = [Guid]::NewGuid().ToString('N')
    $p = Start-Process -FilePath $ps -ArgumentList $args -WorkingDirectory $root -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $work "$tag-start.out") -RedirectStandardError (Join-Path $work "$tag-start.err")
    $null = $p.Handle
    if (-not $p.WaitForExit(180000)) {
        Stop-Process -Id $p.Id -ErrorAction SilentlyContinue
        throw 'New installation did not become ready.'
    }
    $p.Refresh()
    if ($p.ExitCode -ne 0) { throw 'Installation startup failed; inspect update logs.' }
}

function Assert-Healthy([string]$Version, [bool]$Probe) {
    for ($i=0; $i -lt 3; $i++) {
        $base = "http://127.0.0.1:$($job.console_port)"
        $health = Invoke-RestMethod "$base/api/health" -TimeoutSec 5
        $auth = Invoke-RestMethod "$base/api/auth/status" -TimeoutSec 5
        if (-not $health.ready -or $health.version -ne $Version -or $health.service -ne 'yunkun' -or
            $auth.initialized -isnot [bool] -or [bool]$health.update_probe -ne $Probe) {
            throw 'Version, database or authentication readiness check failed.'
        }
        Start-Sleep -Milliseconds 500
    }
}

try {
    Assert-Within $work $updates
    if ([IO.Path]::GetFullPath($JobFile) -ne (Join-Path $work 'job.json')) { throw 'Unexpected job path.' }
    $lock = [IO.File]::Open((Join-Path $updates 'update.lock'), 'OpenOrCreate', 'ReadWrite', 'None')
    $parent = Get-Process -Id $job.parent_pid -ErrorAction Stop
    if ($parent.Path -ne $app) { throw 'Parent process is not the managed console.' }
    $size = [long](Invoke-Files 'check')
    $drive = Get-PSDrive -Name ([IO.Path]::GetPathRoot($root).TrimEnd('\').TrimEnd(':'))
    if ($drive.Free -lt ($size * 2 + 1GB)) { throw 'Insufficient disk space for verified backup and rollback.' }
    Set-State 'waiting' 'Update verified. Waiting for the console to shut down.'
    [IO.File]::WriteAllText((Join-Path $work 'ready'), 'ready')
    if (-not $parent.WaitForExit(120000)) { throw 'Console did not stop; no installation files were replaced.' }
    $stopped = $true
    Stop-Database
    Set-State 'backing_up' 'Backing up the stopped database, configuration, keys and installation.'
    Invoke-Files 'backup'
    $backedUp = $true
    [IO.File]::WriteAllText((Join-Path $work 'backup-verified'), 'verified')
    Set-State 'installing' 'Backup verified. Installing the new version.'
    $changed = $true
    Invoke-Files 'install'
    Set-State 'starting' 'Checking database migrations and authentication before accepting traffic.'
    Start-Installation $true
    Assert-Healthy $job.to $true
    Stop-App
    Start-Installation $false
    Assert-Healthy $job.to $false
    Set-State 'completed' 'Update completed. Previous data and a full backup were retained.'
} catch {
    $failure = $_.Exception.Message
    Write-Host $failure
    if (-not $lock) { exit 1 }
    try {
        if ($changed -and $backedUp) {
            Set-State 'rolling_back' 'Startup failed. Restoring the matching program and database snapshot.' $failure
            Stop-App
            Stop-Database
            Invoke-Files 'restore'
        }
        if ($stopped) {
            Start-Installation $false
            Assert-Healthy $job.from $false
        }
        $phase = if ($changed -and $backedUp) { 'rolled_back' } else { 'failed' }
        Set-State $phase 'Update did not complete. The previous installation was retained.' $failure
    } catch {
        Set-State 'recovery_required' 'Automatic recovery stopped. Keep the backup and failed installation; do not initialize a new database.' `
            ($failure + '; recovery: ' + $_.Exception.Message)
    }
    exit 1
} finally {
    if ($lock) { $lock.Dispose() }
}
