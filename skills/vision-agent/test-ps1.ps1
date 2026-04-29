param(
  [string]$ApiBase = "http://127.0.0.1:18789"
)

$ErrorActionPreference = "Stop"

function Invoke-AnyClawTool {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Tool,

    [Parameter(Mandatory = $true)]
    [hashtable]$Params
  )

  $body = $Params | ConvertTo-Json -Depth 8
  Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/tools/$Tool" -ContentType "application/json" -Body $body
}

function Test-Screenshot {
  Write-Host "Capturing screenshot..."
  Invoke-AnyClawTool -Tool "desktop_screenshot" -Params @{
    path = ".anyclaw/vision/screen.png"
  } | Out-Null
  Write-Host "Screenshot captured."
}

function Test-OCR {
  Write-Host "Running OCR..."
  $result = Invoke-AnyClawTool -Tool "desktop_ocr" -Params @{
    path = ".anyclaw/vision/screen.png"
  }
  $text = [string]$result.text
  if ($text.Length -gt 120) {
    $text = $text.Substring(0, 120)
  }
  Write-Host "OCR sample: $text"
}

function Test-OpenNotepad {
  Write-Host "Opening Notepad..."
  Invoke-AnyClawTool -Tool "desktop_open" -Params @{
    target = "notepad.exe"
    kind = "app"
  } | Out-Null
  Invoke-AnyClawTool -Tool "desktop_wait" -Params @{
    wait_ms = 1000
  } | Out-Null
  Write-Host "Open request sent."
}

function Test-TypeDemoText {
  Write-Host "Typing demo text..."
  Invoke-AnyClawTool -Tool "desktop_type_human" -Params @{
    text = "Hello from AnyClaw vision-agent"
    delay_ms = 30
  } | Out-Null
  Write-Host "Demo text typed."
}

Write-Host "AnyClaw vision-agent smoke helpers"
Write-Host "1. Screenshot"
Write-Host "2. OCR"
Write-Host "3. Open Notepad"
Write-Host "4. Type demo text"
Write-Host "0. Exit"

$choice = Read-Host "Choose an option"
switch ($choice) {
  "1" { Test-Screenshot }
  "2" { Test-OCR }
  "3" { Test-OpenNotepad }
  "4" { Test-TypeDemoText }
  "0" { exit 0 }
  default { Write-Host "Unknown option: $choice" }
}
