[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Candidate,
    [Parameter(Mandatory)][string]$TestRoot
)
$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $TestRoot) { throw 'Use a new isolated fixture directory.' }
$root = Join-Path ([IO.Path]::GetFullPath($TestRoot)) 'installation'
$private = Join-Path $root '.yunkun-local'
$app = Join-Path $root 'bin\yunkun.exe'
$pg = Join-Path $root 'runtime\pgsql-18.4\bin'
$port = 53533
$dbPort = 55463
$base = "http://127.0.0.1:$port"
$checks = @()
$launchers = @()
foreach ($p in @($port, $dbPort)) {
    if (Get-NetTCPConnection -State Listen -LocalPort $p -ErrorAction SilentlyContinue) {
        throw "Fixture port occupied: $p"
    }
}
if (Test-Path -LiteralPath (Join-Path $Candidate '.yunkun-local')) { throw 'Candidate contains private state.' }
New-Item -ItemType Directory -Path $TestRoot | Out-Null
Copy-Item -LiteralPath $Candidate -Destination $root -Recurse
$ps = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'

function Start-Fixture([string]$Label) {
    $scriptFile = Join-Path $root 'setup-and-start-offline.ps1'
    $p = Start-Process -FilePath $ps -WindowStyle Hidden -PassThru `
        -ArgumentList @('-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
            '-File', ('"' + $scriptFile + '"'), '-NoBrowser', '-ConsolePort', "$port", '-DatabasePort', "$dbPort") `
        -RedirectStandardOutput (Join-Path $TestRoot "$Label.out") `
        -RedirectStandardError (Join-Path $TestRoot "$Label.err")
    $null = $p.Handle
    $script:launchers += $p
    if (-not $p.WaitForExit(240000)) { throw 'Fixture launcher timed out.' }
    $p.Refresh()
    if ($p.ExitCode -ne 0) { throw "Fixture startup failed; inspect $Label.out and $Label.err." }
    $health = Invoke-RestMethod "$base/api/health" -TimeoutSec 5
    if ($health.version -ne 'v1.1.0-offline' -or -not $health.ready -or $health.service -ne 'yunkun') {
        throw 'Unexpected readiness identity.'
    }
}

function Stop-Fixture {
    $runFile = Join-Path $private 'process.json'
    if (Test-Path -LiteralPath $runFile) {
        $run = Get-Content -LiteralPath $runFile -Raw | ConvertFrom-Json
        $p = Get-Process -Id $run.pid -ErrorAction SilentlyContinue
        if ($p) {
            if ($p.Path -ne $app) { throw 'Unexpected application process owner.' }
            Stop-Process -Id $p.Id
            Wait-Process -Id $p.Id -Timeout 15 -ErrorAction SilentlyContinue
        }
    }
    $data = Join-Path $private 'postgres-data'
    $pidFile = Join-Path $data 'postmaster.pid'
    if (Test-Path -LiteralPath $pidFile) {
        $lines = @(Get-Content -LiteralPath $pidFile)
        if ($lines.Count -lt 2 -or [IO.Path]::GetFullPath($lines[1]) -ne $data) {
            throw 'Unexpected database data directory.'
        }
        $dbProcess = Get-Process -Id ([int]$lines[0]) -ErrorAction SilentlyContinue
        if ($dbProcess) {
            if ($dbProcess.Path -ne (Join-Path $pg 'postgres.exe')) { throw 'Unexpected database process owner.' }
            $p = Start-Process -FilePath (Join-Path $pg 'pg_ctl.exe') -WindowStyle Hidden -PassThru `
                -ArgumentList @('stop', '-D', ('"' + $data + '"'), '-m', 'fast', '-w', '-t', '60') `
                -RedirectStandardOutput (Join-Path $TestRoot 'stop.out') `
                -RedirectStandardError (Join-Path $TestRoot 'stop.err')
            $null = $p.Handle
            if (-not $p.WaitForExit(75000) -or $p.ExitCode -ne 0) { throw 'Fixture database stop failed.' }
        }
    }
    foreach ($p in @($port, $dbPort)) {
        if (Get-NetTCPConnection -State Listen -LocalPort $p -ErrorAction SilentlyContinue) {
            throw "Fixture listener remains: $p"
        }
    }
}

try {
    Start-Fixture 'fresh'
    $checks += 'fresh PostgreSQL init and matching application health'
    $html = (Invoke-WebRequest "$base/login/" -UseBasicParsing -TimeoutSec 5).Content
    if ($html -notmatch '<html' -or $html -notmatch '_next/') { throw 'Embedded frontend missing.' }
    if ((Invoke-RestMethod "$base/api/auth/status").initialized) { throw 'Candidate contains an administrator.' }
    $checks += 'embedded frontend and uninitialized administrator'
    $password = [Guid]::NewGuid().ToString('N') + 'Aa1'
    $auth = Invoke-RestMethod "$base/api/auth/init" -Method Post -ContentType application/json `
        -Body (@{password=$password} | ConvertTo-Json)
    if (-not $auth.token) { throw 'Initialization did not return an authenticated session.' }
    $headers = @{Authorization='Bearer ' + $auth.token}
    $null = Invoke-RestMethod "$base/api/auth/login" -Method Post -ContentType application/json `
        -Body (@{username='yunkun';password=$password} | ConvertTo-Json)
    $tasks = Invoke-RestMethod "$base/api/tasks" -Headers $headers
    if (@($tasks.tasks).Where({ $null -ne $_ }).Count -or
        ($tasks -is [array] -and $tasks.Count)) { throw 'Candidate contains customer tasks.' }
    $checks += 'first administrator login and empty task history'
    $kb = Invoke-RestMethod "$base/api/rag/knowledge-bases" -Method Post -Headers $headers -ContentType application/json `
        -Body (@{name='deployment-synthetic-only';visibility='private';chunk_size=900;chunk_overlap=120} | ConvertTo-Json)
    if (-not $kb.id) { throw 'Synthetic knowledge base creation failed.' }
    $stateFile = Join-Path $private 'installation.json'
    $configFile = Join-Path $private 'config.json'
    $stateHash = (Get-FileHash -LiteralPath $stateFile).Hash
    $configHash = (Get-FileHash -LiteralPath $configFile).Hash
    $pidBefore = (Get-Content -LiteralPath (Join-Path $private 'process.json') -Raw | ConvertFrom-Json).pid
    Start-Fixture 'already-running'
    if ((Get-Content -LiteralPath (Join-Path $private 'process.json') -Raw | ConvertFrom-Json).pid -ne $pidBefore) {
        throw 'Idempotent start replaced an existing process.'
    }
    $checks += 'repeat start reuses ready owned process'
    Stop-Fixture
    Start-Fixture 'restart'
    if ((Get-FileHash -LiteralPath $stateFile).Hash -ne $stateHash -or
        (Get-FileHash -LiteralPath $configFile).Hash -ne $configHash) { throw 'Restart changed private state/config.' }
    $null = Invoke-RestMethod "$base/api/auth/me" -Headers $headers
    $null = Invoke-RestMethod "$base/api/auth/login" -Method Post -ContentType application/json `
        -Body (@{username='yunkun';password=$password} | ConvertTo-Json)
    if ((Invoke-RestMethod "$base/api/rag/knowledge-bases" -Headers $headers | ConvertTo-Json -Depth 8) -notmatch 'deployment-synthetic-only') {
        throw 'Restart lost synthetic knowledge.'
    }
    $checks += 'restart retains credentials session config and synthetic knowledge'
    Stop-Fixture
    $checks += 'owned test listeners stopped; database retained'
    @{ok=$true;version='v1.1.0-offline';platform='windows-amd64';checks=$checks;
        clean_vm=$false;vc_missing_branch_tested=$false} |
        ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $TestRoot 'result.json') -Encoding utf8
    Write-Host 'PASS: isolated Windows fresh deployment and persistence checks.'
} finally {
    foreach ($p in $launchers) {
        if (-not $p.HasExited) {
            $scriptPath = (Get-CimInstance Win32_Process -Filter "ProcessId=$($p.Id)").CommandLine
            if ($scriptPath -like "*$root*") { Stop-Process -Id $p.Id }
        }
    }
    Stop-Fixture
}
