package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const SDKVersion = "anyclaw.app.sdk.v1"

type ExecutionContext struct {
	AppID       string            `json:"app_id"`
	Action      string            `json:"action"`
	Params      map[string]any    `json:"params,omitempty"`
	Binding     map[string]string `json:"binding,omitempty"`
	Config      map[string]any    `json:"config,omitempty"`
	State       map[string]any    `json:"state,omitempty"`
	StepIndex   int               `json:"step_index,omitempty"`
	Checkpoints []Checkpoint      `json:"checkpoints,omitempty"`
}

type Checkpoint struct {
	Index    int            `json:"index"`
	Label    string         `json:"label"`
	Time     time.Time      `json:"time"`
	State    map[string]any `json:"state,omitempty"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

type BaseConnector struct {
	Name         string
	ProcessName  string
	WindowTitle  string
	LaunchCmd    string
	Capabilities []string
	Actions      []string
	UIInspect    *UIMap
}

func NewBaseConnector(name, processName, windowTitle, launchCmd string) *BaseConnector {
	return &BaseConnector{
		Name:         name,
		ProcessName:  processName,
		WindowTitle:  windowTitle,
		LaunchCmd:    launchCmd,
		Capabilities: []string{},
		Actions:      []string{},
		UIInspect:    nil,
	}
}

func (b *BaseConnector) GetCapabilities() []string {
	return b.Capabilities
}

func (b *BaseConnector) GetActions() []string {
	return b.Actions
}

func (b *BaseConnector) DefaultCapabilities() []string {
	return []string{
		"window-management",
		"text-input",
		"click-events",
		"screenshot",
		"ocr",
	}
}

func (b *BaseConnector) Probe(ctx context.Context) (*ProbeResult, error) {
	result := &ProbeResult{
		Installed: false,
		Running:   false,
	}

	if runtime.GOOS != "windows" {
		result.Error = "only supported on Windows"
		return result, nil
	}

	if b.LaunchCmd != "" {
		_, err := os.Stat(b.LaunchCmd)
		if err == nil {
			result.Installed = true
			result.Path = b.LaunchCmd
		}
	}

	script := fmt.Sprintf(`
$procName = '%s';
$proc = Get-Process -Name $procName -ErrorAction SilentlyContinue | Select-Object -First 1;
if ($proc) {
	@{
		running = $true;
		pid = $proc.Id;
	} | ConvertTo-Json -Compress
} else {
	@{ running = $false } | ConvertTo-Json -Compress
}
`, b.ProcessName)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err == nil {
		var status struct {
			Running bool `json:"running"`
			PID     int  `json:"pid"`
		}
		if json.Unmarshal(output, &status) == nil {
			result.Running = status.Running
			if status.PID > 0 {
				result.Version = fmt.Sprintf("PID: %d", status.PID)
			}
		}
	}

	return result, nil
}

func (b *BaseConnector) Launch(ctx context.Context, args ...string) (*AppWindowState, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	cmdArgs := []string{}
	if b.LaunchCmd != "" {
		cmdArgs = append(cmdArgs, b.LaunchCmd)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to launch: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	return b.ProbeWindow(ctx)
}

func (b *BaseConnector) Focus(ctx context.Context) (*AppWindowState, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
	[DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
}
"@
$procName = '%s';
$win = Get-Process -Name $procName -ErrorAction SilentlyContinue | Select-Object -First 1;
if ($win -and $win.MainWindowHandle -ne [IntPtr]::Zero) {
	[Win32]::SetForegroundWindow($win.MainWindowHandle) | Out-Null;
	[Win32]::ShowWindow($win.MainWindowHandle, 9) | Out-Null;
	@{ success = $true } | ConvertTo-Json -Compress;
} else {
	@{ success = $false; error = "window not found" } | ConvertTo-Json -Compress;
}
`, b.ProcessName)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	_, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to focus: %w", err)
	}

	return b.ProbeWindow(ctx)
}

func (b *BaseConnector) ProbeWindow(ctx context.Context) (*AppWindowState, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
$procName = '%s';
$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
);
foreach ($w in $windows) {
	$current = $w.Current;
	$handle = [int]$current.NativeWindowHandle;
	if ($handle -eq 0) { continue; }
	$proc = Get-Process -Id $current.ProcessId -ErrorAction SilentlyContinue;
	if ($proc -and $proc.ProcessName -eq $procName) {
		$rect = $current.BoundingRectangle;
		@{
			title = $current.Name;
			process_name = $proc.ProcessName;
			process_id = $current.ProcessId;
			handle = $handle;
			x = [int]$rect.Left;
			y = [int]$rect.Top;
			width = [int]($rect.Right - $rect.Left);
			height = [int]($rect.Bottom - $rect.Top);
			is_focused = [bool]$current.HasKeyboardFocus;
		} | ConvertTo-Json -Compress;
		break;
	}
}
`, b.ProcessName)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("window not found")
	}

	var window AppWindowState
	if err := json.Unmarshal(output, &window); err != nil {
		return nil, fmt.Errorf("failed to parse window: %w", err)
	}

	return &window, nil
}

func (b *BaseConnector) Close(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
$procName = '%s';
$proc = Get-Process -Name $procName -ErrorAction SilentlyContinue | Select-Object -First 1;
if ($proc) { 
	$proc.Kill(); 
	@{ success = $true } | ConvertTo-Json -Compress;
} else {
	@{ success = $false; error = "not running" } | ConvertTo-Json -Compress;
}
`, b.ProcessName)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	_, err := cmd.Output()
	return err
}

func (b *BaseConnector) Screenshot(ctx context.Context, path string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("only supported on Windows")
	}

	window, err := b.ProbeWindow(ctx)
	if err != nil {
		return "", err
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Drawing;
$bounds = New-Object System.Drawing.Rectangle(%d, %d, %d, %d);
$bitmap = New-Object System.Drawing.Bitmap($bounds.Width, $bounds.Height);
$graphics = [System.Drawing.Graphics]::FromImage($bitmap);
$graphics.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size);
$bitmap.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png);
$graphics.Dispose();
$bitmap.Dispose();
@{ path = '%s'; width = $bounds.Width; height = $bounds.Height } | ConvertTo-Json -Compress;
`, window.X, window.Y, window.Width, window.Height, path, path)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (b *BaseConnector) InspectUI(ctx context.Context, maxElements int) (*UIMap, error) {
	app := &AppInfo{
		ID:          strings.ToLower(strings.ReplaceAll(b.Name, " ", "-")),
		Name:        b.Name,
		ProcessName: b.ProcessName,
		WindowTitle: b.WindowTitle,
	}
	return InspectAppUI(ctx, app, maxElements)
}

func (b *BaseConnector) FindElement(ctx context.Context, name, controlType string, index int) (*UIElement, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
$procName = '%s';
$elementName = '%s';
$controlType = '%s';
$index = %d;

Add-Type -AssemblyName UIAutomationClient;
Add-Type -AssemblyName UIAutomationTypes;

$window = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
) | Where-Object { $_.Current.ProcessId -and (Get-Process -Id $_.Current.ProcessId -ErrorAction SilentlyContinue).ProcessName -eq $procName } | Select-Object -First 1;

if (-not $window) { throw "Window not found" }

$elements = $window.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition);
$matches = @();
foreach ($e in $elements) {
	$current = $e.Current;
	$nameMatch = -not $elementName -or $current.Name -like "*$elementName*";
	$typeMatch = -not $controlType -or (([string]$current.ControlType.ProgrammaticName) -replace 'ControlType\\.', '') -eq $controlType;
	if ($nameMatch -and $typeMatch) {
		$rect = $current.BoundingRectangle;
		if ($rect.Width -gt 0 -and $rect.Height -gt 0) {
			$matches += @{
				name = $current.Name;
				automation_id = $current.AutomationId;
				class_name = $current.ClassName;
				control_type = (([string]$current.ControlType.ProgrammaticName) -replace 'ControlType\\.', '');
				x = [int]$rect.Left;
				y = [int]$rect.Top;
				width = [int]$rect.Width;
				height = [int]$rect.Height;
				center_x = [int]($rect.Left + $rect.Width / 2);
				center_y = [int]($rect.Top + $rect.Height / 2);
				is_enabled = $current.IsEnabled;
			};
		}
	}
	if ($matches.Count -ge $index) { break; }
}

if ($matches.Count -ge $index) {
	$matches[$index - 1] | ConvertTo-Json -Compress;
} else {
	@{ error = "element not found" } | ConvertTo-Json -Compress;
}
`, b.ProcessName, name, controlType, index)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to find element: %w", err)
	}

	var element UIElement
	if err := json.Unmarshal(output, &element); err != nil {
		return nil, fmt.Errorf("failed to parse element: %w", err)
	}

	if element.Name == "" && element.AutomationID == "" {
		return nil, fmt.Errorf("element not found: %s", string(output))
	}

	return &element, nil
}

func (b *BaseConnector) ClickElement(ctx context.Context, name, controlType string, index int) error {
	element, err := b.FindElement(ctx, name, controlType, index)
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
	[DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
	[DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, IntPtr dwExtraInfo);
}
"@
[Mouse]::SetCursorPos(%d, %d) | Out-Null;
[Mouse]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero);
[Mouse]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero);
@{ success = $true } | ConvertTo-Json -Compress;
`, element.CenterX, element.CenterY)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	_, err = cmd.Output()
	return err
}

func (b *BaseConnector) TypeText(ctx context.Context, text string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
[System.Windows.Forms.SendKeys]::SendWait('%s');
@{ success = $true } | ConvertTo-Json -Compress;
`, escapeSendKeys(text))

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	_, err := cmd.Output()
	return err
}

func escapeSendKeys(text string) string {
	replacer := strings.NewReplacer(
		"{", "{{}",
		"}", "{}}",
		"+", "{+}",
		"^", "{^}",
		"%", "{%}",
		"~", "{~}",
		"(", "{(}",
		")", "{)}",
	)
	return replacer.Replace(text)
}

func (b *BaseConnector) Hotkey(ctx context.Context, keys ...string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
[System.Windows.Forms.SendKeys]::SendWait('^a');
Start-Sleep -Milliseconds 50;
@{ success = $true } | ConvertTo-Json -Compress;
`)

	if len(keys) == 1 {
		script = fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
[System.Windows.Forms.SendKeys]::SendWait('{%s}');
@{ success = $true } | ConvertTo-Json -Compress;
`, strings.ToUpper(keys[0]))
	} else {
		var sendKeys []string
		for _, k := range keys {
			sendKeys = append(sendKeys, "^"+strings.ToUpper(k))
		}
		script = fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms;
[System.Windows.Forms.SendKeys]::SendWait('%s');
@{ success = $true } | ConvertTo-Json -Compress;
`, strings.Join(sendKeys, ""))
	}

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	_, err := cmd.Output()
	return err
}

type StepBuilder struct {
	connector *BaseConnector
	Steps     []DesktopPlanStep
}

func NewStepBuilder(connector *BaseConnector) *StepBuilder {
	return &StepBuilder{
		connector: connector,
		Steps:     []DesktopPlanStep{},
	}
}

func (sb *StepBuilder) Launch(args ...string) *StepBuilder {
	launchCmd := sb.connector.LaunchCmd
	if len(args) > 0 {
		launchCmd = args[0]
	}
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_open",
		Label: "Launch " + sb.connector.Name,
		Input: map[string]any{
			"target": launchCmd,
			"kind":   "app",
		},
		Retry:       1,
		WaitAfterMS: 600,
	})
	return sb
}

func (sb *StepBuilder) WaitWindow(timeoutMS int) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_wait_window",
		Label: "Wait for " + sb.connector.Name + " window",
		Input: map[string]any{
			"title":        sb.connector.WindowTitle,
			"process_name": sb.connector.ProcessName,
			"match":        "contains",
			"timeout_ms":   timeoutMS,
		},
		Retry:        1,
		RetryDelayMS: 400,
	})
	return sb
}

func (sb *StepBuilder) Focus() *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_focus_window",
		Label: "Focus " + sb.connector.Name + " window",
		Input: map[string]any{
			"title":        sb.connector.WindowTitle,
			"process_name": sb.connector.ProcessName,
			"match":        "contains",
		},
		Retry:        2,
		RetryDelayMS: 300,
	})
	return sb
}

func (sb *StepBuilder) Click(name string, index int) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Label: "Click " + name,
		Target: map[string]any{
			"title":        sb.connector.WindowTitle,
			"process_name": sb.connector.ProcessName,
			"name":         name,
			"index":        index,
		},
		Action:      "click",
		Retry:       1,
		WaitAfterMS: 300,
	})
	return sb
}

func (sb *StepBuilder) Type(name, value string, submit bool) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Label: "Type into " + name,
		Target: map[string]any{
			"title":        sb.connector.WindowTitle,
			"process_name": sb.connector.ProcessName,
			"name":         name,
		},
		Value:       &value,
		Submit:      &submit,
		Retry:       1,
		WaitAfterMS: 250,
	})
	return sb
}

func (sb *StepBuilder) Hotkey(keys ...string) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_hotkey",
		Label: "Press " + strings.Join(keys, "+"),
		Input: map[string]any{
			"keys": keys,
		},
		WaitAfterMS: 100,
	})
	return sb
}

func (sb *StepBuilder) Wait(waitMS int) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_wait",
		Label: fmt.Sprintf("Wait %dms", waitMS),
		Input: map[string]any{"wait_ms": waitMS},
	})
	return sb
}

func (sb *StepBuilder) Screenshot(path string) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_screenshot_window",
		Label: "Screenshot",
		Input: map[string]any{
			"path":         path,
			"title":        sb.connector.WindowTitle,
			"process_name": sb.connector.ProcessName,
			"match":        "contains",
		},
	})
	return sb
}

func (sb *StepBuilder) OCR(path, lang string) *StepBuilder {
	input := map[string]any{"path": path}
	if lang != "" {
		input["lang"] = lang
	}
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_ocr",
		Label: "OCR",
		Input: input,
	})
	return sb
}

func (sb *StepBuilder) VerifyText(expected string, mode string) *StepBuilder {
	sb.Steps = append(sb.Steps, DesktopPlanStep{
		Tool:  "desktop_verify_text",
		Label: "Verify text",
		Input: map[string]any{
			"path":        ".anyclaw/screenshot.png",
			"expected":    expected,
			"mode":        mode,
			"ignore_case": true,
		},
		Retry:        2,
		RetryDelayMS: 500,
	})
	return sb
}

func (sb *StepBuilder) OnFailure(steps ...DesktopPlanStep) *StepBuilder {
	if len(sb.Steps) > 0 {
		sb.Steps[len(sb.Steps)-1].OnFailure = steps
	}
	return sb
}

func (sb *StepBuilder) ContinueOnError() *StepBuilder {
	if len(sb.Steps) > 0 {
		sb.Steps[len(sb.Steps)-1].ContinueOnError = true
	}
	return sb
}

func (sb *StepBuilder) Build() *DesktopPlan {
	return &DesktopPlan{
		Protocol: DesktopProtocolVersion,
		Summary:  sb.connector.Name + " automation plan",
		Steps:    sb.Steps,
	}
}

func GenerateConnectorCode(appName, processName, windowTitle, launchCmd string) string {
	safeName := strings.ReplaceAll(appName, " ", "")
	safeName = strings.ReplaceAll(safeName, "+", "")

	return fmt.Sprintf(`package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anyclaw/anyclaw/pkg/apps"
)

type %sConnector struct {
	*apps.BaseConnector
}

func New%sConnector() *%sConnector {
	connector := apps.NewBaseConnector(
		"%s",
		"%s",
		"%s",
		"%s",
	)
	connector.Capabilities = connector.DefaultCapabilities()
	connector.Actions = []string{
		"open",
		"close",
		"send-keys",
		"screenshot",
		"ocr",
	}
	return &%sConnector{BaseConnector: connector}
}

func (c *%sConnector) Probe(ctx context.Context) (*apps.ProbeResult, error) {
	return c.BaseConnector.Probe(ctx)
}

func (c *%sConnector) Execute(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	switch action {
	case "open":
		return c.executeOpen(ctx, params)
	case "close":
		return c.executeClose(ctx, params)
	case "send-keys":
		return c.executeSendKeys(ctx, params)
	case "screenshot":
		return c.executeScreenshot(ctx, params)
	case "ocr":
		return c.executeOCR(ctx, params)
	default:
		return nil, fmt.Errorf("unknown action: %%s", action)
	}
}

func (c *%sConnector) executeOpen(ctx context.Context, params map[string]any) (map[string]any, error) {
	window, err := c.BaseConnector.Launch(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"success":      true,
		"window_title": window.Title,
		"process_id":   window.ProcessID,
	}, nil
}

func (c *%sConnector) executeClose(ctx context.Context, params map[string]any) (map[string]any, error) {
	err := c.BaseConnector.Close(ctx)
	return map[string]any{"success": err == nil}, err
}

func (c *%sConnector) executeSendKeys(ctx context.Context, params map[string]any) (map[string]any, error) {
	text, _ := params["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("text parameter required")
	}
	err := c.BaseConnector.TypeText(ctx, text)
	return map[string]any{"success": err == nil}, err
}

func (c *%sConnector) executeScreenshot(ctx context.Context, params map[string]any) (map[string]any, error) {
	path, _ := params["path"].(string)
	if path == "" {
		path = ".anyclaw/screenshots/%%s.png"
	}
	result, err := c.BaseConnector.Screenshot(ctx, path)
	return map[string]any{"success": err == nil, "result": result}, err
}

func (c *%sConnector) executeOCR(ctx context.Context, params map[string]any) (map[string]any, error) {
	path := ".anyclaw/ocr.png"
	if p, ok := params["path"].(string); ok {
		path = p
	}
	lang := "chi_sim+eng"
	if l, ok := params["lang"].(string); ok {
		lang = l
	}
	_, err := c.BaseConnector.Screenshot(ctx, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"success":    true,
		"path":       path,
		"lang":       lang,
		"note":       "Use desktop_ocr tool for actual OCR",
	}, nil
}

func (c *%sConnector) GetCapabilities() []string {
	return c.BaseConnector.GetCapabilities()
}

func (c *%sConnector) GetActions() []string {
	return c.BaseConnector.GetActions()
}

func main() {
	connector := New%sConnector()
	
	ctx := context.Background()
	
	result, err := connector.Probe(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Probe error: %%v\\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("App: %%s\\n", connector.Name)
	fmt.Printf("Installed: %%v\\n", result.Installed)
	fmt.Printf("Running: %%v\\n", result.Running)
	
	if result.Running {
		window, err := connector.BaseConnector.ProbeWindow(ctx)
		if err == nil {
			fmt.Printf("Window: %%s (%%dx%%d)\\n", window.Title, window.Width, window.Height)
		}
	}
}
`,
		safeName, safeName, safeName,
		appName, processName, windowTitle, launchCmd,
		safeName,
		safeName, safeName, safeName, safeName, safeName, safeName,
		safeName, safeName, safeName, safeName,
	)
}

func GeneratePluginTemplate(appName, processName, windowTitle, launchCmd, category string) string {
	safeName := strings.ReplaceAll(appName, " ", "")
	safeName = strings.ReplaceAll(safeName, "+", "")
	id := strings.ToLower(strings.ReplaceAll(appName, " ", "-"))

	return fmt.Sprintf(`{
  "api_version": "v2",
  "plugin_id": "%s",
  "name": "%s",
  "version": "1.0.0",
  "description": "App Connector for %s",
  "author": "AnyClaw Team",
  "license": "MIT",
  "kinds": ["app"],
  "builtin": false,
  "enabled": true,
  "entrypoint": "app.py",
  "exec_policy": "manual-allow",
  "timeout_seconds": 30,
  "platforms": ["windows"],
  "capability_tags": ["desktop-control", "ui-automation", "%s"],
  "trigger_words": ["%s", "%s"],
  "risk_level": "medium",
  "data_access": ["file-system", "clipboard", "ui-state"],
  "approval_scope": "action",
  "app": {
    "name": "%s",
    "description": "App Connector for %s",
    "transport": "desktop",
    "platforms": ["windows"],
    "capabilities": [
      "window-management",
      "text-input", 
      "click-events",
      "screenshot",
      "ocr"
    ],
    "desktop": {
      "launch_command": "%s",
      "window_title": "%s",
      "focus_strategy": "title-contains"
    },
    "actions": [
      {
        "name": "open",
        "description": "Open %s",
        "kind": "execute"
      },
      {
        "name": "close", 
        "description": "Close %s",
        "kind": "execute"
      },
      {
        "name": "screenshot",
        "description": "Take a screenshot",
        "kind": "execute"
      }
    ]
  }
}
`,
		id, appName, appName, category,
		strings.ToLower(appName), strings.ReplaceAll(appName, " ", ""),
		appName, appName,
		launchCmd, windowTitle,
		appName, appName,
	)
}
