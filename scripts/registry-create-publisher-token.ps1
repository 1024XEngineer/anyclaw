param(
  [string]$BaseUrl = "http://127.0.0.1:8791",

  [Parameter(Mandatory = $true)]
  [string]$AdminToken,

  [Parameter(Mandatory = $true)]
  [string]$PublisherId
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

$uri = "$(Resolve-RegistryApiBase -Value $BaseUrl)/admin/tokens"
$body = @{ publisher_id = $PublisherId } | ConvertTo-Json -Depth 5

Invoke-RestMethod -Method Post -Uri $uri `
  -Headers @{ Authorization = "Bearer $AdminToken" } `
  -ContentType "application/json" `
  -Body $body
