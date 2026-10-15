param(
    [ValidateSet('Process', 'User', 'Machine')]
    [string]$Scope = 'Process',
    [switch]$Apply,
    [switch]$RemoveLegacy
)
$ErrorActionPreference = 'Stop'
$names = @(
    'CONFIG', 'SKILL_DIR', 'PG_DSN',
    'LLM_PROVIDER', 'LLM_BASE_URL', 'LLM_MODEL', 'LLM_PROXY', 'LLM_STREAM',
    'BTW_MAX_OUTPUT_TOKENS', 'NOTIFY_ALLOW_LOCAL'
)
foreach ($name in $names) {
    $oldName = 'ARTEX_' + $name
    $newName = 'YUNKUN_' + $name
    $oldValue = [Environment]::GetEnvironmentVariable($oldName, $Scope)
    $current = [Environment]::GetEnvironmentVariable($newName, $Scope)
    if ([string]::IsNullOrWhiteSpace($oldValue)) { continue }
    if (-not $Apply) {
        $action = if ([string]::IsNullOrWhiteSpace($current)) { 'Would migrate' } else { 'Would keep current' }
        Write-Output "$action $newName ($Scope); values are hidden"
        continue
    }
    if ([string]::IsNullOrWhiteSpace($current)) {
        [Environment]::SetEnvironmentVariable($newName, $oldValue, $Scope)
        if ([Environment]::GetEnvironmentVariable($newName, $Scope) -cne $oldValue) {
            throw "Write verification failed for $newName; legacy value is preserved."
        }
    }
    if ($RemoveLegacy) {
        [Environment]::SetEnvironmentVariable($oldName, $null, $Scope)
    }
    Write-Output "Configured $newName ($Scope); values are hidden"
}
