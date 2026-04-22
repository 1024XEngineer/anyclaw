package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var desktopHostOS = runtime.GOOS
var desktopScriptRunner = runDesktopPowerShell

func DesktopOpenTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	target, ok := input["target"].(string)
	if !ok || strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("target is required")
	}
	kind, _ := input["kind"].(string)
	if err := ensureDesktopAllowed("desktop_open", opts, false); err != nil {
		return "", err
	}
	if kind == "file" || kind == "app" {
		if err := validateProtectedPath(target, opts.ProtectedPaths); err != nil {
			return "", err
		}
	}
	command := desktopOpenCommand(strings.TrimSpace(target), strings.TrimSpace(kind))
	return desktopScriptRunner(ctx, command)
}

func DesktopTypeTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	text, ok := input["text"].(string)
	if !ok || text == "" {
		return "", fmt.Errorf("text is required")
	}
	if err := ensureDesktopAllowed("desktop_type", opts, false); err != nil {
		return "", err
	}
	command := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(%s); "typed"`, powerShellString(sendKeysEscape(text)))
	return desktopScriptRunner(ctx, command)
}

func DesktopTypeHumanTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	text, ok := input["text"].(string)
	if !ok || text == "" {
		return "", fmt.Errorf("text is required")
	}
	if err := ensureDesktopAllowed("desktop_type_human", opts, false); err != nil {
		return "", err
	}
	delayMS, ok := numberInput(input["delay_ms"])
	if !ok || delayMS < 0 {
		delayMS = 45
	}
	jitterMS, ok := numberInput(input["jitter_ms"])
	if !ok || jitterMS < 0 {
		jitterMS = 35
	}
	pauseEvery, ok := numberInput(input["pause_every"])
	if !ok || pauseEvery < 0 {
		pauseEvery = 18
	}
	pauseMS, ok := numberInput(input["pause_ms"])
	if !ok || pauseMS < 0 {
		pauseMS = 220
	}
	submit, _ := input["submit"].(bool)
	parts := make([]string, 0, len([]rune(text)))
	for _, r := range []rune(text) {
		parts = append(parts, sendKeysEscape(string(r)))
	}
	payload, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	command := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
$items = ConvertFrom-Json %s;
$baseDelay = %d;
$jitterDelay = %d;
$pauseEvery = %d;
$pauseDelay = %d;
$submit = %s;
$rand = New-Object System.Random;
for ($i = 0; $i -lt $items.Count; $i++) {
  [System.Windows.Forms.SendKeys]::SendWait([string]$items[$i]);
  $sleep = $baseDelay;
  if ($jitterDelay -gt 0) {
    $sleep += $rand.Next(0, $jitterDelay + 1);
  }
  if ($pauseEvery -gt 0 -and (($i + 1) %% $pauseEvery) -eq 0) {
    $sleep += $pauseDelay;
  }
  if ($sleep -gt 0) {
    Start-Sleep -Milliseconds $sleep;
  }
}
if ($submit) {
  Start-Sleep -Milliseconds 90;
  [System.Windows.Forms.SendKeys]::SendWait("{ENTER}");
}
"typed human"
`, powerShellString(string(payload)), delayMS, jitterMS, pauseEvery, pauseMS, powerShellBool(submit))
	return desktopScriptRunner(ctx, command)
}

func DesktopHotkeyTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	keys, _ := input["keys"].([]any)
	if len(keys) == 0 {
		if single, ok := input["keys"].([]string); ok && len(single) > 0 {
			keys = make([]any, 0, len(single))
			for _, item := range single {
				keys = append(keys, item)
			}
		}
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("keys is required")
	}
	if err := ensureDesktopAllowed("desktop_hotkey", opts, false); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(keys))
	for _, item := range keys {
		parts = append(parts, fmt.Sprint(item))
	}
	command := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(%s); "hotkey sent"`, powerShellString(hotkeyToSendKeys(parts)))
	return desktopScriptRunner(ctx, command)
}

func DesktopClipboardSetTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	text, ok := input["text"].(string)
	if !ok {
		return "", fmt.Errorf("text is required")
	}
	if err := ensureDesktopAllowed("desktop_clipboard_set", opts, false); err != nil {
		return "", err
	}
	command := fmt.Sprintf(`
Set-Clipboard -Value %s;
"clipboard set"
`, powerShellString(text))
	return desktopScriptRunner(ctx, command)
}

func DesktopClipboardGetTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_clipboard_get", opts, true); err != nil {
		return "", err
	}
	command := `
$text = "";
try {
  $raw = Get-Clipboard -Raw -TextFormatType Text -ErrorAction Stop;
  if ($null -ne $raw) {
    $text = [string]$raw;
  }
} catch {
  Add-Type -AssemblyName System.Windows.Forms;
  $text = [System.Windows.Forms.Clipboard]::GetText();
}
$text
`
	return desktopScriptRunner(ctx, command)
}

func DesktopPasteTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	text, _ := input["text"].(string)
	submit, _ := input["submit"].(bool)
	waitMS, ok := numberInput(input["wait_ms"])
	if !ok || waitMS < 0 {
		waitMS = 90
	}
	if err := ensureDesktopAllowed("desktop_paste", opts, false); err != nil {
		return "", err
	}
	setClipboard := ""
	if text != "" {
		setClipboard = fmt.Sprintf("Set-Clipboard -Value %s;", powerShellString(text))
	}
	command := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
%s
if (%d -gt 0) {
  Start-Sleep -Milliseconds %d;
}
[System.Windows.Forms.SendKeys]::SendWait("^v");
if (%s) {
  Start-Sleep -Milliseconds 90;
  [System.Windows.Forms.SendKeys]::SendWait("{ENTER}");
}
"pasted"
`, setClipboard, waitMS, waitMS, powerShellBool(submit))
	return desktopScriptRunner(ctx, command)
}

func DesktopClickTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	x, okX := numberInput(input["x"])
	y, okY := numberInput(input["y"])
	if !okX || !okY {
		return "", fmt.Errorf("x and y are required")
	}
	if err := ensureDesktopAllowed("desktop_click", opts, false); err != nil {
		return "", err
	}
	button := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["button"])))
	if button == "" {
		button = "left"
	}
	if boolValue(input["human_like"]) {
		command, err := desktopHumanClickCommand(
			x,
			y,
			button,
			false,
			intNumberWithDefault(input["duration_ms"], 280),
			intNumberWithDefault(input["steps"], 18),
			intNumberWithDefault(input["jitter_px"], 3),
			intNumberWithDefault(input["settle_ms"], 70),
			0,
		)
		if err != nil {
			return "", err
		}
		return desktopScriptRunner(ctx, command)
	}
	downFlag, upFlag, err := mouseFlags(button)
	if err != nil {
		return "", err
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@;
[DesktopNative]::SetCursorPos(%d, %d) | Out-Null;
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
"clicked"
`, x, y, downFlag, upFlag)
	return desktopScriptRunner(ctx, command)
}

func DesktopMoveTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	x, okX := numberInput(input["x"])
	y, okY := numberInput(input["y"])
	if !okX || !okY {
		return "", fmt.Errorf("x and y are required")
	}
	if err := ensureDesktopAllowed("desktop_move", opts, false); err != nil {
		return "", err
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
}
"@;
[DesktopNative]::SetCursorPos(%d, %d) | Out-Null;
"moved"
`, x, y)
	return desktopScriptRunner(ctx, command)
}

func DesktopDoubleClickTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	x, okX := numberInput(input["x"])
	y, okY := numberInput(input["y"])
	if !okX || !okY {
		return "", fmt.Errorf("x and y are required")
	}
	if err := ensureDesktopAllowed("desktop_double_click", opts, false); err != nil {
		return "", err
	}
	button := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["button"])))
	if button == "" {
		button = "left"
	}
	intervalMS, ok := numberInput(input["interval_ms"])
	if !ok || intervalMS <= 0 {
		intervalMS = 120
	}
	if boolValue(input["human_like"]) {
		command, err := desktopHumanClickCommand(
			x,
			y,
			button,
			true,
			intNumberWithDefault(input["duration_ms"], 320),
			intNumberWithDefault(input["steps"], 20),
			intNumberWithDefault(input["jitter_px"], 3),
			intNumberWithDefault(input["settle_ms"], 80),
			intervalMS,
		)
		if err != nil {
			return "", err
		}
		return desktopScriptRunner(ctx, command)
	}
	downFlag, upFlag, err := mouseFlags(button)
	if err != nil {
		return "", err
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@;
[DesktopNative]::SetCursorPos(%d, %d) | Out-Null;
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
Start-Sleep -Milliseconds %d;
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
"double-clicked"
`, x, y, downFlag, upFlag, intervalMS, downFlag, upFlag)
	return desktopScriptRunner(ctx, command)
}

func DesktopScrollTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_scroll", opts, false); err != nil {
		return "", err
	}
	delta, ok := numberInput(input["delta"])
	if !ok || delta == 0 {
		direction := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["direction"])))
		clicks, okClicks := numberInput(input["clicks"])
		if !okClicks || clicks <= 0 {
			clicks = 3
		}
		delta = clicks * 120
		if direction == "down" {
			delta = -delta
		}
		if direction != "" && direction != "up" && direction != "down" {
			return "", fmt.Errorf("direction must be up or down")
		}
	}
	x, hasX := numberInput(input["x"])
	y, hasY := numberInput(input["y"])
	if hasX != hasY {
		return "", fmt.Errorf("x and y must be provided together")
	}
	moveCursor := ""
	if hasX && hasY {
		moveCursor = fmt.Sprintf("[DesktopNative]::SetCursorPos(%d, %d) | Out-Null;", x, y)
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@;
%s
[DesktopNative]::mouse_event(0x0800, 0, 0, %d, [UIntPtr]::Zero);
"scrolled"
`, moveCursor, delta)
	return desktopScriptRunner(ctx, command)
}

func DesktopDragTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	x1, okX1 := numberInput(input["x1"])
	y1, okY1 := numberInput(input["y1"])
	x2, okX2 := numberInput(input["x2"])
	y2, okY2 := numberInput(input["y2"])
	if !okX1 || !okY1 || !okX2 || !okY2 {
		return "", fmt.Errorf("x1, y1, x2, and y2 are required")
	}
	if err := ensureDesktopAllowed("desktop_drag", opts, false); err != nil {
		return "", err
	}
	button := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["button"])))
	if button == "" {
		button = "left"
	}
	steps, ok := numberInput(input["steps"])
	if !ok || steps <= 0 {
		steps = 12
	}
	durationMS, ok := numberInput(input["duration_ms"])
	if !ok || durationMS <= 0 {
		durationMS = 300
	}
	delay := durationMS / steps
	if delay <= 0 {
		delay = 1
	}
	downFlag, upFlag, err := mouseFlags(button)
	if err != nil {
		return "", err
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@;
[DesktopNative]::SetCursorPos(%d, %d) | Out-Null;
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
for ($i = 1; $i -le %d; $i++) {
  $x = [int][Math]::Round(%d + ((%d - %d) * $i / %d));
  $y = [int][Math]::Round(%d + ((%d - %d) * $i / %d));
  [DesktopNative]::SetCursorPos($x, $y) | Out-Null;
  Start-Sleep -Milliseconds %d;
}
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
"dragged"
`, x1, y1, downFlag, steps, x1, x2, x1, steps, y1, y2, y1, steps, delay, upFlag)
	return desktopScriptRunner(ctx, command)
}

func DesktopWaitTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	waitMS, ok := numberInput(input["wait_ms"])
	if !ok || waitMS < 0 {
		return "", fmt.Errorf("wait_ms is required")
	}
	if err := ensureDesktopAllowed("desktop_wait", opts, false); err != nil {
		return "", err
	}
	command := fmt.Sprintf(`Start-Sleep -Milliseconds %d; "waited"`, waitMS)
	return desktopScriptRunner(ctx, command)
}

func DesktopFocusWindowTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	title, _ := input["title"].(string)
	processName, _ := input["process_name"].(string)
	if strings.TrimSpace(title) == "" && strings.TrimSpace(processName) == "" {
		return "", fmt.Errorf("title or process_name is required")
	}
	if err := ensureDesktopAllowed("desktop_focus_window", opts, false); err != nil {
		return "", err
	}
	match := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["match"])))
	if match == "" {
		match = "contains"
	}
	if match != "contains" && match != "exact" {
		return "", fmt.Errorf("match must be contains or exact")
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool ShowWindowAsync(IntPtr hWnd, int nCmdShow);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
}
"@;
$title = %s;
$processName = %s;
$match = %s;
$target = $null;
if ($title -ne "") {
  $windows = Get-Process | Where-Object { $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle -ne "" };
  if ($match -eq "exact") {
    $target = $windows | Where-Object { $_.MainWindowTitle -eq $title } | Select-Object -First 1;
  } else {
    $target = $windows | Where-Object { $_.MainWindowTitle -like ("*" + $title + "*") } | Select-Object -First 1;
  }
}
if (-not $target -and $processName -ne "") {
  $target = Get-Process -Name $processName -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1;
}
if (-not $target) {
  throw "window not found";
}
[DesktopNative]::ShowWindowAsync($target.MainWindowHandle, 5) | Out-Null;
Start-Sleep -Milliseconds 100;
if (-not [DesktopNative]::SetForegroundWindow($target.MainWindowHandle)) {
  $wshell = New-Object -ComObject WScript.Shell;
  $null = $wshell.AppActivate($target.Id);
}
"focused"
`, powerShellString(strings.TrimSpace(title)), powerShellString(strings.TrimSpace(processName)), powerShellString(match))
	return desktopScriptRunner(ctx, command)
}

func DesktopScreenshotTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	path, ok := input["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := ensureDesktopAllowed("desktop_screenshot", opts, true); err != nil {
		return "", err
	}
	resolved := resolvePath(path, opts.WorkingDir)
	if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	if err := ensureWriteAllowed(resolved, opts.WorkingDir, opts.PermissionLevel); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("failed to create screenshot dir: %w", err)
	}
	output, err := captureDesktopScreenshotToPath(ctx, resolved)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s to %s", strings.TrimSpace(output), resolved), nil
}

func captureDesktopScreenshotToPath(ctx context.Context, resolved string) (string, error) {
	command := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
Add-Type -AssemblyName System.Drawing;
$bounds = [System.Windows.Forms.SystemInformation]::VirtualScreen;
$bitmap = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height;
$graphics = [System.Drawing.Graphics]::FromImage($bitmap);
$graphics.CopyFromScreen($bounds.Left, $bounds.Top, 0, 0, $bitmap.Size);
$bitmap.Save(%s, [System.Drawing.Imaging.ImageFormat]::Png);
$graphics.Dispose();
$bitmap.Dispose();
"saved"
`, powerShellString(resolved))
	return desktopScriptRunner(ctx, command)
}

func ensureDesktopAllowed(toolName string, opts BuiltinOptions, allowReadOnly bool) error {
	if desktopHostOS != "windows" {
		return fmt.Errorf("%s is currently supported on Windows host mode only", toolName)
	}
	mode := strings.TrimSpace(strings.ToLower(opts.ExecutionMode))
	if mode != "host-reviewed" {
		return fmt.Errorf("%s requires sandbox.execution_mode=host-reviewed", toolName)
	}
	if !allowReadOnly && strings.TrimSpace(strings.ToLower(opts.PermissionLevel)) == "read-only" {
		return fmt.Errorf("permission denied: current agent is read-only")
	}
	return nil
}

func runDesktopPowerShell(ctx context.Context, script string) (string, error) {
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	cmd, err := shellCommandWithShell(ctx, fmt.Sprintf("[Text.Encoding]::Unicode.GetString([Convert]::FromBase64String('%s')) | Invoke-Expression", encoded), "powershell")
	if err != nil {
		return "", err
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("desktop action failed: %w - %s", err, string(output))
	}
	return string(output), nil
}

func utf16LE(s string) []byte {
	runes := []rune(s)
	buf := make([]byte, 0, len(runes)*2)
	for _, r := range runes {
		if r > 0xFFFF {
			r = '?'
		}
		buf = append(buf, byte(r), byte(r>>8))
	}
	return buf
}

func desktopOpenCommand(target string, kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "url":
		return fmt.Sprintf(`Start-Process %s; "opened url"`, powerShellString(target))
	case "file":
		return fmt.Sprintf(`Invoke-Item %s; "opened file"`, powerShellString(target))
	case "app":
		return fmt.Sprintf(`Start-Process %s; "started app"`, powerShellString(target))
	default:
		return fmt.Sprintf(`Start-Process %s; "opened target"`, powerShellString(target))
	}
}

func powerShellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func sendKeysEscape(text string) string {
	replacer := strings.NewReplacer(
		"{", "{{}",
		"}", "{}}",
		"+", "{+}",
		"^", "{^}",
		"%", "{%}",
		"~", "{~}",
		"(", "{(}",
		")", "{)}",
		"[", "{[}",
		"]", "{]}",
	)
	return replacer.Replace(text)
}

func hotkeyToSendKeys(keys []string) string {
	modifiers := ""
	plain := make([]string, 0, len(keys))
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "ctrl", "control":
			modifiers += "^"
		case "alt":
			modifiers += "%"
		case "shift":
			modifiers += "+"
		case "win", "windows", "meta":
			plain = append(plain, "{LWIN}")
		default:
			plain = append(plain, formatSendKey(strings.TrimSpace(key)))
		}
	}
	return modifiers + strings.Join(plain, "")
}

func formatSendKey(key string) string {
	if len(key) == 1 {
		return sendKeysEscape(key)
	}
	upper := strings.ToUpper(strings.TrimSpace(key))
	switch upper {
	case "ENTER", "TAB", "ESC", "ESCAPE", "UP", "DOWN", "LEFT", "RIGHT", "BACKSPACE", "DELETE", "HOME", "END", "PGUP", "PGDN":
		return "{" + upper + "}"
	default:
		return "{" + upper + "}"
	}
}

func mouseFlags(button string) (int, int, error) {
	switch button {
	case "left":
		return 0x0002, 0x0004, nil
	case "right":
		return 0x0008, 0x0010, nil
	case "middle":
		return 0x0020, 0x0040, nil
	default:
		return 0, 0, fmt.Errorf("unsupported button: %s", button)
	}
}

func desktopHumanClickCommand(x int, y int, button string, doubleClick bool, durationMS int, steps int, jitterPX int, settleMS int, intervalMS int) (string, error) {
	if durationMS <= 0 {
		durationMS = 280
	}
	if steps <= 0 {
		steps = 18
	}
	if jitterPX < 0 {
		jitterPX = 0
	}
	if settleMS < 0 {
		settleMS = 0
	}
	if intervalMS <= 0 {
		intervalMS = 120
	}
	downFlag, upFlag, err := mouseFlags(button)
	if err != nil {
		return "", err
	}
	delay := durationMS / steps
	if delay <= 0 {
		delay = 1
	}
	command := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public struct DesktopPoint {
  public int X;
  public int Y;
}
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out DesktopPoint lpPoint);
}
"@;
$targetX = %d;
$targetY = %d;
$steps = %d;
$delay = %d;
$jitter = %d;
$settle = %d;
$doubleClick = %s;
$interval = %d;
$rand = New-Object System.Random;
$point = New-Object DesktopPoint;
[DesktopNative]::GetCursorPos([ref]$point) | Out-Null;
$startX = [int]$point.X;
$startY = [int]$point.Y;
if ($jitter -gt 0) {
  $targetX += $rand.Next(-$jitter, $jitter + 1);
  $targetY += $rand.Next(-$jitter, $jitter + 1);
}
for ($i = 1; $i -le $steps; $i++) {
  $x = [int][Math]::Round($startX + (($targetX - $startX) * $i / $steps));
  $y = [int][Math]::Round($startY + (($targetY - $startY) * $i / $steps));
  [DesktopNative]::SetCursorPos($x, $y) | Out-Null;
  Start-Sleep -Milliseconds $delay;
}
if ($settle -gt 0) {
  Start-Sleep -Milliseconds $settle;
}
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
[DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
if ($doubleClick) {
  Start-Sleep -Milliseconds $interval;
  [DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
  [DesktopNative]::mouse_event(%d, 0, 0, 0, [UIntPtr]::Zero);
  "double-clicked human"
} else {
  "clicked human"
}
`, x, y, steps, delay, jitterPX, settleMS, powerShellBool(doubleClick), intervalMS, downFlag, upFlag, downFlag, upFlag)
	return command, nil
}

func intNumberWithDefault(value any, fallback int) int {
	n, ok := numberInput(value)
	if !ok {
		return fallback
	}
	return n
}

func powerShellBool(value bool) string {
	if value {
		return "$true"
	}
	return "$false"
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func numberInput(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case string:
		var out int
		_, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &out)
		return out, err == nil
	default:
		return 0, false
	}
}
