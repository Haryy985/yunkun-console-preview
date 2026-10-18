[CmdletBinding()]
param(
    [string]$Baseline = 'F:\AI\ARTEX-main(1)\.tools\publish-v1.0.2-20261016-final\yunkun-v1.0.2-offline',
    [string]$Candidate = 'F:\YunkunOffline\v1.0.3-offline-complete-20261018\yunkun-v1.0.3-offline',
    [string]$TestRoot = 'F:\YunkunOffline\.build\release103-upgrade'
)
$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $TestRoot) { throw 'Fixture already exists.' }
$old = Join-Path $TestRoot 'old'
$new = Join-Path $TestRoot 'new'
$backup = Join-Path $TestRoot 'backup'
New-Item -ItemType Directory -Path $TestRoot | Out-Null
Copy-Item -LiteralPath $Baseline -Destination $old -Recurse
Copy-Item -LiteralPath $Candidate -Destination $new -Recurse
# Only use the new launcher's configurable port to keep the live console untouched.
# The old program and runtime remain the actual 1.0.2 release artifacts.
Copy-Item -LiteralPath (Join-Path $Candidate 'setup-and-start-offline.ps1') -Destination $old -Force

function Stop-Fixture([string]$Root) {
    $runFile = Join-Path $Root '.yunkun-local\process.json'
    if (Test-Path -LiteralPath $runFile) {
        $run = Get-Content -LiteralPath $runFile -Raw | ConvertFrom-Json
        $process = Get-Process -Id $run.pid -ErrorAction SilentlyContinue
        if ($process) {
            if ($process.Path -ne (Join-Path $Root 'bin\yunkun.exe')) { throw 'Unexpected app owner.' }
            Stop-Process -Id $run.pid
            Wait-Process -Id $run.pid -Timeout 10 -ErrorAction SilentlyContinue
        }
    }
    $data = Join-Path $Root '.yunkun-local\postgres-data'
    $pidFile = Join-Path $data 'postmaster.pid'
    if (Test-Path -LiteralPath $pidFile) {
        $pgProcessId = [int](Get-Content -LiteralPath $pidFile -TotalCount 1)
        if ((Get-Process -Id $pgProcessId).Path -ne (Join-Path $Root 'runtime\pgsql-18.4\bin\postgres.exe')) {
            throw 'Unexpected PostgreSQL owner.'
        }
        $control = Start-Process -FilePath (Join-Path $Root 'runtime\pgsql-18.4\bin\pg_ctl.exe') `
            -ArgumentList @('-D', ('"' + $data + '"'), '-m', 'fast', '-w', 'stop') `
            -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $TestRoot 'stop.out') `
            -RedirectStandardError (Join-Path $TestRoot 'stop.err')
        if (-not $control.WaitForExit(30000)) { throw 'Fixture database did not stop.' }
    }
}

$base = 'http://127.0.0.1:53513'
try {
    & (Join-Path $old 'setup-and-start-offline.ps1') -NoBrowser -ConsolePort 53513 -DatabasePort 55435
    if ($LASTEXITCODE -ne 0) { throw 'Old release startup failed.' }
    if ((Invoke-RestMethod "$base/api/health").version -ne 'v1.0.2-offline') { throw 'Wrong baseline.' }
    $password = [Guid]::NewGuid().ToString('N') + 'Fixture'
    $identity = Invoke-RestMethod "$base/api/auth/init" -Method Post -ContentType application/json `
        -Body (@{ password=$password } | ConvertTo-Json)
    $headers = @{ Authorization='Bearer ' + $identity.token }
    $knowledge = Invoke-RestMethod "$base/api/rag/knowledge-bases" -Method Post -Headers $headers `
        -ContentType application/json -Body (@{ name='upgrade-knowledge-fixture'; visibility='private'; chunk_size=900; chunk_overlap=120 } | ConvertTo-Json)
    $task = Invoke-RestMethod "$base/api/tasks" -Method Post -Headers $headers -ContentType application/json `
        -Body (@{ description='upgrade-history-fixture'; goal='Local fixture; no target'; seed_first_intent=$false; rag_knowledge_base_ids=@($knowledge.id) } | ConvertTo-Json)
    $pool = Invoke-RestMethod "$base/api/proxy-pools" -Method Post -Headers $headers -ContentType application/json `
        -Body (@{ config=@{ name='upgrade-proxy-fixture'; kind='fixed'; enabled=$false; addresses=@('http://127.0.0.1:9'); safety_seconds=1 };
            secrets=@{ proxy_username='fixture'; proxy_password='fixture-only-no-real-secret' } } | ConvertTo-Json -Depth 6)
    New-Item -ItemType Directory -Path (Join-Path $old 'skills\upgrade-custom') -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $old 'skills\upgrade-custom\SKILL.md'), "---`nname: upgrade-custom`ndescription: Test fixture`n---`nUser retained content.")
    $keyHashes = @{}
    foreach ($key in @('jwt.key','proxy-pool.key')) {
        $keyHashes[$key] = (Get-FileHash -LiteralPath (Join-Path $old ('bin\' + $key))).Hash
    }
    Stop-Fixture $old
    & (Join-Path $new 'upgrade-offline.ps1') -OldDirectory $old -BackupDirectory $backup
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed.' }
    foreach ($key in @('jwt.key','proxy-pool.key')) {
        if ((Get-FileHash -LiteralPath (Join-Path $new ('bin\' + $key))).Hash -ne $keyHashes[$key]) {
            throw 'Private key changed.'
        }
    }
    & (Join-Path $new 'setup-and-start-offline.ps1') -NoBrowser -ConsolePort 53513
    if ($LASTEXITCODE -ne 0) { throw 'New release startup failed.' }
    if ((Invoke-RestMethod "$base/api/health").version -ne 'v1.0.3-offline') { throw 'Wrong upgraded version.' }
    # Prove both old session and password work after the migration.
    $null = Invoke-RestMethod "$base/api/auth/me" -Headers $headers
    $null = Invoke-RestMethod "$base/api/auth/login" -Method Post -ContentType application/json `
        -Body (@{ username='yunkun'; password=$password } | ConvertTo-Json)
    $tasks = Invoke-RestMethod "$base/api/tasks" -Headers $headers
    if (($tasks | ConvertTo-Json -Depth 8) -notmatch 'upgrade-history-fixture') { throw 'Task history missing.' }
    $knowledgeBases = Invoke-RestMethod "$base/api/rag/knowledge-bases" -Headers $headers
    if (($knowledgeBases | ConvertTo-Json -Depth 8) -notmatch 'upgrade-knowledge-fixture') { throw 'Knowledge base missing.' }
    $pools = Invoke-RestMethod "$base/api/proxy-pools" -Headers $headers
    $retained = $pools.pools | Where-Object { $_.id -eq $pool.id }
    if (-not $retained.has_credentials) { throw 'Proxy credentials could not be decrypted.' }
    if (-not (Test-Path -LiteralPath (Join-Path $new 'skills\upgrade-custom\SKILL.md'))) { throw 'Custom Skill missing.' }
    $visibility = Invoke-RestMethod "$base/api/agents/worker/visibility" -Headers $headers
    if ($visibility.skill -notcontains 'yunkun-security-workflow') { throw 'New workflow visibility missing.' }
    @{ baseline='v1.0.2-offline'; upgraded='v1.0.3-offline'; taskHistory=$true; knowledgeBase=$true;
        sessionAndPassword=$true; decryptProxyCredentials=$true; customSkills=$true; newWorkflow=$true } |
        ConvertTo-Json | Set-Content -LiteralPath (Join-Path $TestRoot 'upgrade-result.json') -Encoding utf8
    Write-Host 'Real 1.0.2 -> 1.0.3 upgrade passed: history, knowledge base, sessions, encrypted proxy credentials and Skills retained.'
} finally {
    Stop-Fixture $old
    Stop-Fixture $new
}
