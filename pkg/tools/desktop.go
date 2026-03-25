package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

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
	return runDesktopPowerShell(ctx, command)
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
	return runDesktopPowerShell(ctx, command)
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
	return runDesktopPowerShell(ctx, command)
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
	return runDesktopPowerShell(ctx, command)
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
	output, err := runDesktopPowerShell(ctx, command)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s to %s", strings.TrimSpace(output), resolved), nil
}

func ensureDesktopAllowed(toolName string, opts BuiltinOptions, allowReadOnly bool) error {
	if runtime.GOOS != "windows" {
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
