param(
  [string]$DataDir = ".anyclaw-registry",

  [string]$OutputDir = "backups/registry",

  [string]$Name = ""
)

$ErrorActionPreference = "Stop"

function Resolve-SqliteCommand {
  $cmd = Get-Command sqlite3 -ErrorAction SilentlyContinue
  if ($cmd) {
    return $cmd.Source
  }
  return $null
}

$dataPath = Resolve-Path -LiteralPath $DataDir
$timestamp = if ([string]::IsNullOrWhiteSpace($Name)) { Get-Date -Format "yyyyMMdd-HHmmss" } else { $Name }
$backupRoot = Join-Path $OutputDir $timestamp
$dbPath = Join-Path $dataPath "registry.db"
$packagesPath = Join-Path $dataPath "packages"
$auditPath = Join-Path $dataPath "audit"

if (-not (Test-Path -LiteralPath $dbPath -PathType Leaf)) {
  throw "Registry database not found: $dbPath"
}

New-Item -ItemType Directory -Force -Path $backupRoot | Out-Null

$dbBackup = Join-Path $backupRoot "registry.db"
$sqlite = Resolve-SqliteCommand
if ($sqlite) {
  & $sqlite $dbPath ".backup '$dbBackup'"
  if ($LASTEXITCODE -ne 0) {
    throw "sqlite3 backup failed with exit code $LASTEXITCODE"
  }
}
else {
  Copy-Item -LiteralPath $dbPath -Destination $dbBackup -Force
}

if (Test-Path -LiteralPath $packagesPath -PathType Container) {
  Copy-Item -LiteralPath $packagesPath -Destination (Join-Path $backupRoot "packages") -Recurse -Force
}
if (Test-Path -LiteralPath $auditPath -PathType Container) {
  Copy-Item -LiteralPath $auditPath -Destination (Join-Path $backupRoot "audit") -Recurse -Force
}

$manifest = [ordered]@{
  created_at = (Get-Date).ToUniversalTime().ToString("o")
  source_data_dir = $dataPath.Path
  backup_dir = (Resolve-Path -LiteralPath $backupRoot).Path
  sqlite_backup_method = if ($sqlite) { "sqlite3 .backup" } else { "file-copy fallback" }
  files = @{
    registry_db = "registry.db"
    packages = if (Test-Path -LiteralPath (Join-Path $backupRoot "packages")) { "packages/" } else { $null }
    audit = if (Test-Path -LiteralPath (Join-Path $backupRoot "audit")) { "audit/" } else { $null }
  }
}

$manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $backupRoot "manifest.json") -Encoding UTF8
Write-Host "Registry backup ready: $backupRoot"
