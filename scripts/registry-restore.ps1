param(
  [Parameter(Mandatory = $true)]
  [string]$BackupDir,

  [Parameter(Mandatory = $true)]
  [string]$TargetDataDir,

  [switch]$Force
)

$ErrorActionPreference = "Stop"

$backupPath = Resolve-Path -LiteralPath $BackupDir
$dbBackup = Join-Path $backupPath "registry.db"
if (-not (Test-Path -LiteralPath $dbBackup -PathType Leaf)) {
  throw "Backup database not found: $dbBackup"
}

if ((Test-Path -LiteralPath $TargetDataDir) -and -not $Force) {
  $existing = Get-ChildItem -LiteralPath $TargetDataDir -Force -ErrorAction SilentlyContinue
  if ($existing.Count -gt 0) {
    throw "TargetDataDir is not empty. Pass -Force to replace it: $TargetDataDir"
  }
}

if (Test-Path -LiteralPath $TargetDataDir) {
  Remove-Item -LiteralPath $TargetDataDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $TargetDataDir | Out-Null

Copy-Item -LiteralPath $dbBackup -Destination (Join-Path $TargetDataDir "registry.db") -Force

$packagesBackup = Join-Path $backupPath "packages"
if (Test-Path -LiteralPath $packagesBackup -PathType Container) {
  Copy-Item -LiteralPath $packagesBackup -Destination (Join-Path $TargetDataDir "packages") -Recurse -Force
}

$auditBackup = Join-Path $backupPath "audit"
if (Test-Path -LiteralPath $auditBackup -PathType Container) {
  Copy-Item -LiteralPath $auditBackup -Destination (Join-Path $TargetDataDir "audit") -Recurse -Force
}

Write-Host "Registry restore ready: $TargetDataDir"
