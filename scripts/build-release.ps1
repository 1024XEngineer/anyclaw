param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern("^[a-z0-9]+-[a-z0-9]+$")]
  [string]$Target,

  [string]$Version = "dev",

  [string]$OutputDir = "dist/release"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$targetParts = $Target.Split("-", 2)
$goos = $targetParts[0]
$goarch = $targetParts[1]
$isWindowsTarget = $goos -eq "windows"
$binarySuffix = if ($isWindowsTarget) { ".exe" } else { "" }
$packageName = "anyclaw_${Version}_${goos}_${goarch}"
$stageDir = Join-Path $repoRoot ".release/$packageName"
$releaseDir = Join-Path $repoRoot $OutputDir

function Invoke-ExternalCommand {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Description,

    [Parameter(Mandatory = $true)]
    [scriptblock]$Command
  )

  & $Command
  if ($LASTEXITCODE -ne 0) {
    throw "$Description failed with exit code $LASTEXITCODE."
  }
}

function Reset-Directory {
  param([Parameter(Mandatory = $true)][string]$Path)

  if (Test-Path -LiteralPath $Path) {
    Remove-Item -LiteralPath $Path -Recurse -Force
  }
  New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

Push-Location $repoRoot
try {
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found in PATH."
  }

  Reset-Directory -Path $stageDir
  New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

  $env:GOOS = $goos
  $env:GOARCH = $goarch
  $env:CGO_ENABLED = "0"

  $ldflags = "-s -w -X main.version=$Version"
  Invoke-ExternalCommand -Description "Building anyclaw for $Target" -Command {
    go build -trimpath -ldflags $ldflags -o (Join-Path $stageDir "anyclaw$binarySuffix") ./cmd/anyclaw
  }
  Invoke-ExternalCommand -Description "Building anyclaw-registry for $Target" -Command {
    go build -trimpath -ldflags $ldflags -o (Join-Path $stageDir "anyclaw-registry$binarySuffix") ./cmd/anyclaw-registry
  }

  @"
AnyClaw $Version

Target: $Target

Quickstart:
  anyclaw onboard
  anyclaw doctor
  anyclaw -i

Registry:
  anyclaw-registry serve --addr :8791 --data-dir .anyclaw-registry --admin-token <token>
"@ | Set-Content -LiteralPath (Join-Path $stageDir "README.txt") -Encoding UTF8

  if ($isWindowsTarget) {
    $archivePath = Join-Path $releaseDir "$packageName.zip"
    if (Test-Path -LiteralPath $archivePath) {
      Remove-Item -LiteralPath $archivePath -Force
    }
    $archiveItems = Get-ChildItem -LiteralPath $stageDir -Force
    Compress-Archive -LiteralPath $archiveItems.FullName -DestinationPath $archivePath -CompressionLevel Optimal
  }
  else {
    $archivePath = Join-Path $releaseDir "$packageName.tar.gz"
    if (Test-Path -LiteralPath $archivePath) {
      Remove-Item -LiteralPath $archivePath -Force
    }
    Push-Location (Split-Path -Parent $stageDir)
    try {
      Invoke-ExternalCommand -Description "Packing $packageName" -Command {
        tar -czf $archivePath $packageName
      }
    }
    finally {
      Pop-Location
    }
  }

  $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
  "$hash  $(Split-Path -Leaf $archivePath)" | Set-Content -LiteralPath "$archivePath.sha256" -Encoding ASCII

  Write-Host "Release asset ready: $archivePath"
  Write-Host "Checksum ready: $archivePath.sha256"
}
finally {
  Pop-Location
}
