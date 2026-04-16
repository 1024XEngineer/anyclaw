$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$projectDir = Join-Path $repoRoot "cmd/anyclaw-desktop"
$wailsBinary = Join-Path (go env GOPATH) "bin\\wails.exe"

if (-not (Test-Path $wailsBinary)) {
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
}

Push-Location $projectDir
try {
  & $wailsBinary build

  $binDir = Join-Path $projectDir "build\\bin"
  if (-not (Test-Path $binDir)) {
    throw "Desktop build output not found: $binDir"
  }

  $runtimeFolders = @("dist", "skills", "plugins", "workflows")
  foreach ($name in $runtimeFolders) {
    $source = Join-Path $repoRoot $name
    $target = Join-Path $binDir $name
    if (-not (Test-Path $source)) {
      continue
    }
    if (Test-Path $target) {
      Remove-Item -LiteralPath $target -Recurse -Force
    }
    Copy-Item -LiteralPath $source -Destination $target -Recurse -Force
  }

  $configPath = Join-Path $repoRoot "anyclaw.json"
  if (Test-Path $configPath) {
    Copy-Item -LiteralPath $configPath -Destination (Join-Path $binDir "anyclaw.json") -Force
  }

  Write-Host "Desktop build ready:" (Join-Path $projectDir "build\\bin")
}
finally {
  Pop-Location
}
