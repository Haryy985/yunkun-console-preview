[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Candidate,
    [string]$Baseline = 'F:\YunkunOffline\v1.0.6-offline-complete-20261018-final\yunkun-v1.0.6-offline',
    [Parameter(Mandatory)][string]$TestRoot
)
$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $TestRoot) { throw 'Use a new isolated test directory.' }
$root = Join-Path $TestRoot 'installation'
$port = 53531
$dbPort = 55461
$base = "http://127.0.0.1:$port"
foreach ($p in @($port,$dbPort)) {
    if (Get-NetTCPConnection -State Listen -LocalPort $p -ErrorAction SilentlyContinue) { throw 'Fixture port occupied.' }
}
New-Item -ItemType Directory -Path $TestRoot | Out-Null
Copy-Item -LiteralPath $Baseline -Destination $root -Recurse
# Keep the real 1.0.6 executable. The configurable launcher isolates its ports.
Copy-Item -LiteralPath (Join-Path $Candidate 'setup-and-start-offline.ps1') -Destination $root -Force
$private = Join-Path $root '.yunkun-local'
$app = Join-Path $root 'bin\yunkun.exe'
$target = (& (Join-Path $Candidate 'bin\yunkun.exe') -version).Trim()
$processes = @()

function Stop-TestApp {
    $f = Join-Path $private 'process.json'
    if (Test-Path -LiteralPath $f) {
        $run = Get-Content -LiteralPath $f -Raw | ConvertFrom-Json
        $p = Get-Process -Id $run.pid -ErrorAction SilentlyContinue
        if ($p) {
            if ($p.Path -ne $app) { throw 'Unexpected application owner.' }
            Stop-Process -Id $p.Id
            Wait-Process -Id $p.Id -Timeout 15 -ErrorAction SilentlyContinue
        }
    }
}

function Stop-TestDB {
    $data = Join-Path $private 'postgres-data'
    $f = Join-Path $data 'postmaster.pid'
    if (Test-Path -LiteralPath $f) {
        $id = [int](Get-Content -LiteralPath $f -TotalCount 1)
        if ((Get-Process -Id $id).Path -ne (Join-Path $root 'runtime\pgsql-18.4\bin\postgres.exe')) { throw 'Unexpected database owner.' }
        $p = Start-Process -FilePath (Join-Path $root 'runtime\pgsql-18.4\bin\pg_ctl.exe') `
            -ArgumentList @('stop','-D',('"' + $data + '"'),'-m','fast','-w','-t','60') -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput (Join-Path $TestRoot 'stop.out') -RedirectStandardError (Join-Path $TestRoot 'stop.err')
        if (-not $p.WaitForExit(75000)) { throw 'Test database did not stop.' }
    }
}

function Run-Update([string]$Name, [string]$From, [bool]$Fail) {
    $work = Join-Path $private "updates\$Name"
    New-Item -ItemType Directory -Path $work -Force | Out-Null
    Copy-Item -LiteralPath $Candidate -Destination (Join-Path $work 'candidate') -Recurse
    if ($Fail) {
        [IO.File]::WriteAllText((Join-Path $work 'candidate\setup-and-start-offline.ps1'), "throw 'Intentional isolated startup failure'")
    }
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot '..\selfupdate\offline-worker.ps1') -Destination (Join-Path $work 'worker.ps1')
    Copy-Item -LiteralPath (Join-Path $Candidate 'bin\yunkun.exe') -Destination (Join-Path $work 'file-helper.exe')
    $run = Get-Content -LiteralPath (Join-Path $private 'process.json') -Raw | ConvertFrom-Json
    $job = @{ root=$root; work=$work; parent_pid=$run.pid; from=$From; to=$target;
        console_port=$port; database_port=$dbPort; public=$false }
    $jobFile = Join-Path $work 'job.json'
    [IO.File]::WriteAllText($jobFile, ($job | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    $ps = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $p = Start-Process -FilePath $ps -ArgumentList @('-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass',
        '-File',('"' + (Join-Path $work 'worker.ps1') + '"'),'-JobFile',('"' + $jobFile + '"')) `
        -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $work 'worker.out') `
        -RedirectStandardError (Join-Path $work 'worker.err')
    $null = $p.Handle
    $script:processes += $p
    $limit = [DateTime]::UtcNow.AddSeconds(45)
    while (-not (Test-Path -LiteralPath (Join-Path $work 'ready'))) {
        if ($p.HasExited -or [DateTime]::UtcNow -gt $limit) { throw "Helper failed to prepare: $work" }
        Start-Sleep -Milliseconds 250
    }
    # 1.0.6 has no apply endpoint; this models its one-time transition. All
    # subsequent backup/install/restart/rollback actions are the real supervisor.
    Stop-TestApp
    $limit = [DateTime]::UtcNow.AddMinutes(10)
    while (-not $p.WaitForExit(1000)) {
        if ([DateTime]::UtcNow -gt $limit) { throw 'Fixture update timed out.' }
    }
    $state = Get-Content -LiteralPath (Join-Path $private 'update-state.json') -Raw | ConvertFrom-Json
    $expected = if ($Fail) { 'rolled_back' } else { 'completed' }
    if ($state.phase -ne $expected) { throw "Expected $expected, got $($state.phase): $($state.error)" }
    if (-not (Test-Path -LiteralPath (Join-Path $work 'backup-verified'))) { throw 'Backup was not verified.' }
    return $state
}

try {
    & (Join-Path $root 'setup-and-start-offline.ps1') -NoBrowser -ConsolePort $port -DatabasePort $dbPort
    if ($LASTEXITCODE -ne 0) { throw 'Baseline startup failed.' }
    if ((Invoke-RestMethod "$base/api/health").version -ne 'v1.0.6-offline') { throw 'Unexpected baseline.' }
    $password = [Guid]::NewGuid().ToString('N') + 'Fixture'
    $auth = Invoke-RestMethod "$base/api/auth/init" -Method Post -ContentType application/json `
        -Body (@{password=$password} | ConvertTo-Json)
    $headers = @{Authorization='Bearer ' + $auth.token}
    $kb = Invoke-RestMethod "$base/api/rag/knowledge-bases" -Method Post -Headers $headers -ContentType application/json `
        -Body (@{name='oneclick-knowledge';visibility='private';chunk_size=900;chunk_overlap=120} | ConvertTo-Json)
    $task = Invoke-RestMethod "$base/api/tasks" -Method Post -Headers $headers -ContentType application/json `
        -Body (@{description='oneclick-history';goal='Local fixture only';seed_first_intent=$false} | ConvertTo-Json)
    $pool = Invoke-RestMethod "$base/api/proxy-pools" -Method Post -Headers $headers -ContentType application/json `
        -Body (@{config=@{name='oneclick-proxy';kind='fixed';enabled=$false;addresses=@('http://127.0.0.1:9')};
            secrets=@{proxy_username='fixture';proxy_password='not-a-real-secret'}} | ConvertTo-Json -Depth 6)
    New-Item -ItemType Directory -Path (Join-Path $root 'skills\oneclick-custom') | Out-Null
    [IO.File]::WriteAllText((Join-Path $root 'skills\oneclick-custom\SKILL.md'), "User-owned fixture content.")
    $keyHashes = @{}
    foreach ($key in @('jwt.key','proxy-pool.key')) {
        $keyHashes[$key] = (Get-FileHash -LiteralPath (Join-Path $root ('bin\' + $key))).Hash
    }
    $success = Run-Update 'success' 'v1.0.6-offline' $false
    foreach ($key in $keyHashes.Keys) {
        if ((Get-FileHash -LiteralPath (Join-Path $root ('bin\' + $key))).Hash -ne $keyHashes[$key]) { throw 'Private key changed.' }
    }
    $null = Invoke-RestMethod "$base/api/auth/me" -Headers $headers
    $null = Invoke-RestMethod "$base/api/auth/login" -Method Post -ContentType application/json `
        -Body (@{username='yunkun';password=$password} | ConvertTo-Json)
    if ((Invoke-RestMethod "$base/api/tasks" -Headers $headers | ConvertTo-Json -Depth 8) -notmatch 'oneclick-history') { throw 'Task history missing.' }
    if ((Invoke-RestMethod "$base/api/rag/knowledge-bases" -Headers $headers | ConvertTo-Json -Depth 8) -notmatch 'oneclick-knowledge') { throw 'Knowledge missing.' }
    $pools = Invoke-RestMethod "$base/api/proxy-pools" -Headers $headers
    if (-not ($pools.pools | Where-Object id -eq $pool.id).has_credentials) { throw 'Proxy credentials cannot be decrypted.' }
    if ((Get-Content -LiteralPath (Join-Path $root 'skills\oneclick-custom\SKILL.md') -Raw) -ne 'User-owned fixture content.') { throw 'Skill changed.' }
    $rollback = Run-Update 'failed-start' $target $true
    $null = Invoke-RestMethod "$base/api/auth/me" -Headers $headers
    if ((Invoke-RestMethod "$base/api/health").version -ne $target) { throw 'Rollback version mismatch.' }
    if ((Invoke-RestMethod "$base/api/tasks" -Headers $headers | ConvertTo-Json -Depth 8) -notmatch 'oneclick-history') { throw 'Rollback lost history.' }
    @{baseline='v1.0.6-offline';target=$target;success=$success.phase;failure=$rollback.phase;
        password=$true;session=$true;history=$true;knowledge=$true;proxyCredentials=$true;keys=$true;customSkill=$true} |
        ConvertTo-Json | Set-Content -LiteralPath (Join-Path $TestRoot 'result.json') -Encoding utf8
    Write-Host 'PASS: actual 1.0.6 data upgrade and failed-start database rollback.'
} finally {
    foreach ($p in $processes) {
        if (-not $p.HasExited) { $null = $p.WaitForExit(30000) }
    }
    Stop-TestApp
    Stop-TestDB
}
