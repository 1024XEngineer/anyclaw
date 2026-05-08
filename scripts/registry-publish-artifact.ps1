param(
  [string]$BaseUrl = "http://127.0.0.1:8791",

  [Parameter(Mandatory = $true)]
  [string]$PublisherToken,

  [Parameter(Mandatory = $true)]
  [string]$Manifest
)

$ErrorActionPreference = "Stop"

function Resolve-RegistryApiBase {
  param([string]$Value)

  $trimmed = $Value.TrimEnd("/")
  if ($trimmed.EndsWith("/v1")) {
    return $trimmed
  }
  return "$trimmed/v1"
}

if (-not (Test-Path -LiteralPath $Manifest -PathType Leaf)) {
  throw "Manifest not found: $Manifest"
}

$payload = Get-Content -Raw -Encoding UTF8 -LiteralPath $Manifest
$parsed = $payload | ConvertFrom-Json
if ($null -eq $parsed.artifact) {
  throw "Manifest must contain an 'artifact' object."
}
if ([string]::IsNullOrWhiteSpace($parsed.artifact.id)) {
  throw "Manifest artifact.id is required."
}
if ([string]::IsNullOrWhiteSpace($parsed.artifact.kind)) {
  throw "Manifest artifact.kind is required."
}
if ([string]::IsNullOrWhiteSpace($parsed.artifact.latest_version)) {
  throw "Manifest artifact.latest_version is required."
}

$uri = "$(Resolve-RegistryApiBase -Value $BaseUrl)/publish"
Invoke-RestMethod -Method Post -Uri $uri `
  -Headers @{ Authorization = "Bearer $PublisherToken" } `
  -ContentType "application/json" `
  -Body $payload
