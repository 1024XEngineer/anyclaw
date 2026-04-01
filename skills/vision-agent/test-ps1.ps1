# Vision Agent - PowerShell Quick Test

# Configuration
$API = "http://127.0.0.1:18789"
$ScreenshotPath = ".anyclaw\vision\screen.png"

function Invoke-AnyClawTool {
    param($Tool, $Params)
    $body = $Params | ConvertTo-Json -Compress
    $resp = Invoke-RestMethod -Uri "$API/api/v1/tools/$Tool" -Method POST -Body $body -ContentType "application/json"
    return $resp
}

function Test-Screenshot {
    Write-Host "1. Taking screenshot..." -ForegroundColor Cyan
    $result = Invoke-AnyClawTool -Tool "desktop_screenshot" -Params @{path=$ScreenshotPath}
    Write-Host "   Result: $result" -ForegroundColor Green
    
    Write-Host "2. Running OCR..." -ForegroundColor Cyan
    $ocr = Invoke-AnyClawTool -Tool "desktop_ocr" -Params @{path=$ScreenshotPath}
    Write-Host "   Found text: $($ocr.text.Substring(0, [Math]::Min(200, $ocr.text.Length)))" -ForegroundColor Green
}

function Test-LaunchApp {
    param($AppName)
    Write-Host "1. Opening $AppName..." -ForegroundColor Cyan
    Invoke-AnyClawTool -Tool "desktop_open" -Params @{target=$AppName; kind="app"}
    
    Write-Host "2. Waiting 3 seconds..." -ForegroundColor Cyan
    Start-Sleep -Seconds 3
    
    Write-Host "3. Focusing window..." -ForegroundColor Cyan
    Invoke-AnyClawTool -Tool "desktop_focus_window" -Params @{title=$AppName}
    
    Write-Host "4. Taking verification screenshot..." -ForegroundColor Cyan
    $result = Invoke-AnyClawTool -Tool "desktop_screenshot" -Params @{path=$ScreenshotPath}
    Write-Host "   Result: $result" -ForegroundColor Green
}

function Test-ClickByText {
    param($Text)
    Write-Host "1. Taking screenshot..." -ForegroundColor Cyan
    Invoke-AnyClawTool -Tool "desktop_screenshot" -Params @{path=$ScreenshotPath}
    
    Write-Host "2. Finding text: $Text" -ForegroundColor Cyan
    $result = Invoke-AnyClawTool -Tool "desktop_find_text" -Params @{
        text=$Text
        path=$ScreenshotPath
    }
    
    if ($result.found) {
        Write-Host "   Found at ($($result.x), $($result.y))" -ForegroundColor Green
        Write-Host "3. Clicking..." -ForegroundColor Cyan
        Invoke-AnyClawTool -Tool "desktop_click" -Params @{
            x=$result.center_x
            y=$result.center_y
        }
    } else {
        Write-Host "   Text not found!" -ForegroundColor Red
    }
}

function Test-TypeText {
    param($Text)
    Write-Host "Typing: $Text" -ForegroundColor Cyan
    $result = Invoke-AnyClawTool -Tool "desktop_type_human" -Params @{
        text=$Text
        delay_ms=50
    }
    Write-Host "   Result: $result" -ForegroundColor Green
}

function Test-SendMessage {
    param($App, $Contact, $Message)
    # Open app
    Write-Host "1. Opening $App..." -ForegroundColor Cyan
    Invoke-AnyClawTool -Tool "desktop_open" -Params @{target=$App; kind="app"}
    Start-Sleep -Seconds 3
    
    # Find contact
    Write-Host "2. Finding contact: $Contact" -ForegroundColor Cyan
    Invoke-AnyClawTool -Tool "desktop_screenshot" -Params @{path=$ScreenshotPath}
    $result = Invoke-AnyClawTool -Tool "desktop_find_text" -Params @{
        text=$Contact
        path=$ScreenshotPath
    }
    
    if ($result.found) {
        Write-Host "3. Clicking contact..." -ForegroundColor Cyan
        Invoke-AnyClawTool -Tool "desktop_click" -Params @{
            x=$result.center_x
            y=$result.center_y
        }
        Start-Sleep -Seconds 1
        
        # Type message
        Write-Host "4. Typing message: $Message" -ForegroundColor Cyan
        Invoke-AnyClawTool -Tool "desktop_type_human" -Params @{
            text=$Message
            delay_ms=50
        }
        
        # Find send button
        Write-Host "5. Clicking send..." -ForegroundColor Cyan
        Invoke-AnyClawTool -Tool "desktop_screenshot" -Params @{path=$ScreenshotPath}
        $sendResult = Invoke-AnyClawTool -Tool "desktop_find_text" -Params @{
            text="发送"
            path=$ScreenshotPath
        }
        
        if ($sendResult.found) {
            Invoke-AnyClawTool -Tool "desktop_click" -Params @{
                x=$sendResult.center_x
                y=$sendResult.center_y
            }
            Write-Host "   Message sent!" -ForegroundColor Green
        } else {
            Write-Host "   Send button not found, pressing Enter..." -ForegroundColor Yellow
            Invoke-AnyClawTool -Tool "desktop_hotkey" -Params @{keys=@("enter")}
        }
    } else {
        Write-Host "   Contact not found!" -ForegroundColor Red
    }
}

# Main menu
Write-Host ""
Write-Host "========================================" -ForegroundColor Yellow
Write-Host "  Vision Agent - Quick Test Menu" -ForegroundColor Yellow
Write-Host "========================================" -ForegroundColor Yellow
Write-Host ""
Write-Host "1. Test Screenshot + OCR" -ForegroundColor White
Write-Host "2. Launch Notepad" -ForegroundColor White
Write-Host "3. Launch WeChat" -ForegroundColor White
Write-Host "4. Click by Text (specify)" -ForegroundColor White
Write-Host "5. Type Text (specify)" -ForegroundColor White
Write-Host "6. Send WeChat Message" -ForegroundColor White
Write-Host "0. Exit" -ForegroundColor White
Write-Host ""

$choice = Read-Host "Select option"

switch ($choice) {
    "1" { Test-Screenshot }
    "2" { Test-LaunchApp -AppName "notepad" }
    "3" { Test-LaunchApp -AppName "wechat" }
    "4" { 
        $text = Read-Host "Enter text to click"
        Test-ClickByText -Text $text 
    }
    "5" { 
        $text = Read-Host "Enter text to type"
        Test-TypeText -Text $text 
    }
    "6" { 
        $contact = Read-Host "Contact name (default: 文件传输助手)"
        if ($contact -eq "") { $contact = "文件传输助手" }
        $message = Read-Host "Message content"
        Test-SendMessage -App "wechat" -Contact $contact -Message $message
    }
    "0" { exit }
}
