param(
  [string]$BaseUrl = "http://127.0.0.1:8791",
  [string]$AdminToken = "",
  [string]$ArtifactId = "cloud.skill.release-notes"
)

$ErrorActionPreference = "Stop"

function Invoke-Json {
  param(
    [string]$Method = "GET",
    [string]$Url,
    [object]$Body = $null,
    [hashtable]$Headers = @{}
  )

  $params = @{
    Method = $Method
    Uri = $Url
    Headers = $Headers
  }
  if ($null -ne $Body) {
    $params.Body = ($Body | ConvertTo-Json -Depth 8)
    $params.ContentType = "application/json"
  }
  Invoke-RestMethod @params
}

function Assert-True {
  param(
    [bool]$Condition,
    [string]$Message
  )
  if (-not $Condition) {
    throw $Message
  }
}

$BaseUrl = $BaseUrl.TrimEnd("/")

Write-Host "Checking registry health at $BaseUrl"
$health = Invoke-Json -Url "$BaseUrl/v1/health"
Assert-True ($health.data.status -eq "ok") "health check failed"

Write-Host "Checking artifact list"
$artifacts = Invoke-Json -Url "$BaseUrl/v1/artifacts"
Assert-True (($artifacts.data.items | Measure-Object).Count -gt 0) "artifact list is empty"

Write-Host "Checking artifact detail: $ArtifactId"
$detail = Invoke-Json -Url "$BaseUrl/v1/artifacts/$ArtifactId"
Assert-True ($detail.data.id -eq $ArtifactId) "artifact detail id mismatch"

Write-Host "Checking versions"
$versions = Invoke-Json -Url "$BaseUrl/v1/artifacts/$ArtifactId/versions"
Assert-True (($versions.data.items | Measure-Object).Count -gt 0) "versions list is empty"

Write-Host "Checking resolve"
$resolved = Invoke-Json -Method POST -Url "$BaseUrl/v1/artifacts/$ArtifactId/resolve" -Body @{}
Assert-True ($resolved.data.artifact_id -eq $ArtifactId) "resolve artifact id mismatch"
Assert-True ($resolved.data.download_url -ne "") "resolve did not return download_url"
Assert-True ($resolved.data.checksum_sha256 -ne "") "resolve did not return checksum"

Write-Host "Checking download"
$downloadUrl = $resolved.data.download_url
if ($downloadUrl.StartsWith("/")) {
  $downloadUrl = "$BaseUrl$downloadUrl"
}
$downloadResponse = Invoke-WebRequest -Uri $downloadUrl -Method GET -UseBasicParsing
Assert-True ($downloadResponse.StatusCode -eq 200) "download status was not 200"
Assert-True ($downloadResponse.RawContentLength -gt 0) "download was empty"

Write-Host "Checking admin audit auth behavior"
if ($AdminToken -ne "") {
  $admin = Invoke-Json -Url "$BaseUrl/v1/admin/audit" -Headers @{ Authorization = "Bearer $AdminToken" }
  Assert-True ($null -ne $admin.data) "admin audit did not return data"
} else {
  Write-Host "AdminToken not provided; skipped authenticated admin audit check"
}

Write-Host "Registry smoke test passed"
