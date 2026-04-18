$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$projectDir = Join-Path $repoRoot "cmd/anyclaw-desktop"

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

function Get-GoBinDirectory {
  $goBin = (& go env GOBIN).Trim()
  if (-not [string]::IsNullOrWhiteSpace($goBin)) {
    return $goBin
  }

  $goPath = (& go env GOPATH).Trim()
  if ([string]::IsNullOrWhiteSpace($goPath)) {
    throw "Unable to resolve GOPATH from 'go env GOPATH'."
  }

  return (Join-Path $goPath "bin")
}

function Clear-BrokenProxyOverrides {
  $proxyVars = @("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "GIT_HTTP_PROXY", "GIT_HTTPS_PROXY")
  $brokenProxyValues = @(
    "http://127.0.0.1:9",
    "https://127.0.0.1:9",
    "socks5://127.0.0.1:9"
  )
  $snapshot = @{}

  foreach ($name in $proxyVars) {
    $current = [Environment]::GetEnvironmentVariable($name, "Process")
    if ([string]::IsNullOrWhiteSpace($current)) {
      continue
    }

    $normalized = $current.Trim().ToLowerInvariant()
    if ($brokenProxyValues -contains $normalized) {
      $snapshot[$name] = $current
      Remove-Item "Env:$name" -ErrorAction SilentlyContinue
    }
  }

  return $snapshot
}

function Restore-ProcessVariables {
  param(
    [hashtable]$Snapshot
  )

  if ($null -eq $Snapshot) {
    return
  }

  foreach ($name in $Snapshot.Keys) {
    Set-Item "Env:$name" $Snapshot[$name]
  }
}

function Invoke-WithSafeGoEnvironment {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Description,

    [Parameter(Mandatory = $true)]
    [scriptblock]$Command
  )

  $proxySnapshot = Clear-BrokenProxyOverrides
  $originalGoProxy = [Environment]::GetEnvironmentVariable("GOPROXY", "Process")

  try {
    $proxyCandidates = New-Object System.Collections.Generic.List[string]
    foreach ($candidate in @(
      (& go env GOPROXY).Trim(),
      "https://proxy.golang.org,direct",
      "https://goproxy.cn,direct",
      "direct"
    )) {
      if (-not [string]::IsNullOrWhiteSpace($candidate) -and -not $proxyCandidates.Contains($candidate)) {
        $proxyCandidates.Add($candidate)
      }
    }

    $lastError = $null
    foreach ($candidate in $proxyCandidates) {
      $env:GOPROXY = $candidate
      Write-Host "$Description with GOPROXY=$candidate"
      try {
        Invoke-ExternalCommand -Description $Description -Command $Command
      }
      catch {
        $lastError = $_
        continue
      }
      return
    }

    if ($lastError -ne $null) {
      throw $lastError.Exception
    }
    throw "$Description failed for all configured GOPROXY values."
  }
  finally {
    if ($null -ne $originalGoProxy) {
      $env:GOPROXY = $originalGoProxy
    }
    else {
      Remove-Item Env:GOPROXY -ErrorAction SilentlyContinue
    }
    Restore-ProcessVariables -Snapshot $proxySnapshot
  }
}

function Ensure-WailsBinary {
  $wailsBinary = Join-Path (Get-GoBinDirectory) "wails.exe"
  if (Test-Path $wailsBinary) {
    return $wailsBinary
  }

  Invoke-WithSafeGoEnvironment -Description "Installing Wails CLI" -Command {
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
  }

  if (-not (Test-Path $wailsBinary)) {
    throw "Wails CLI was not installed at '$wailsBinary'."
  }

  return $wailsBinary
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "Go was not found in PATH."
}
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw "npm was not found in PATH."
}

$wailsBinary = Ensure-WailsBinary

Push-Location $projectDir
try {
  Invoke-WithSafeGoEnvironment -Description "Running wails build" -Command {
    & $wailsBinary build
  }

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
