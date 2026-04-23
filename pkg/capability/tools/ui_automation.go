package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type desktopWindowInfo struct {
	Title       string `json:"title,omitempty"`
	ProcessName string `json:"process_name,omitempty"`
	ProcessID   int    `json:"process_id,omitempty"`
	Handle      int    `json:"handle,omitempty"`
	X           int    `json:"x,omitempty"`
	Y           int    `json:"y,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	CenterX     int    `json:"center_x,omitempty"`
	CenterY     int    `json:"center_y,omitempty"`
	IsFocused   bool   `json:"is_focused,omitempty"`
}

type desktopAutomationElement struct {
	Name             string `json:"name,omitempty"`
	AutomationID     string `json:"automation_id,omitempty"`
	ClassName        string `json:"class_name,omitempty"`
	ControlType      string `json:"control_type,omitempty"`
	ProcessID        int    `json:"process_id,omitempty"`
	Handle           int    `json:"handle,omitempty"`
	IsEnabled        bool   `json:"is_enabled,omitempty"`
	HasKeyboardFocus bool   `json:"has_keyboard_focus,omitempty"`
	IsOffscreen      bool   `json:"is_offscreen,omitempty"`
	X                int    `json:"x,omitempty"`
	Y                int    `json:"y,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	CenterX          int    `json:"center_x,omitempty"`
	CenterY          int    `json:"center_y,omitempty"`
}

type desktopAutomationActionResult struct {
	Action  string                    `json:"action,omitempty"`
	Element *desktopAutomationElement `json:"element,omitempty"`
	Value   string                    `json:"value,omitempty"`
	Submit  bool                      `json:"submit,omitempty"`
}

type desktopAutomationSelector struct {
	WindowTitle      string
	ProcessName      string
	Handle           int
	Match            string
	Scope            string
	Index            int
	MaxElements      int
	Name             string
	AutomationID     string
	ClassName        string
	ControlType      string
	IncludeOffscreen bool
	IncludeDisabled  bool
}

const desktopAutomationPSScriptPrelude = `
Add-Type -AssemblyName UIAutomationClient;
Add-Type -AssemblyName UIAutomationTypes;

function Get-WindowProcessName([int]$ProcessId) {
  if ($ProcessId -le 0) {
    return ""
  }
  $proc = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($proc) {
    return [string]$proc.ProcessName
  }
  return ""
}

function Match-DesktopValue([string]$Candidate, [string]$Needle, [string]$MatchMode) {
  if ([string]::IsNullOrWhiteSpace($Needle)) {
    return $true
  }
  if ($null -eq $Candidate) {
    $Candidate = ""
  }
  if ($MatchMode -eq "exact") {
    return $Candidate -eq $Needle
  }
  return $Candidate -like ("*" + $Needle + "*")
}

function Get-ControlTypeName($ControlType) {
  if ($null -eq $ControlType) {
    return ""
  }
  return ([string]$ControlType.ProgrammaticName) -replace '^ControlType\.', ''
}

function Get-BoundsObject($Rect) {
  $width = [int]($Rect.Right - $Rect.Left)
  $height = [int]($Rect.Bottom - $Rect.Top)
  [pscustomobject]@{
    x = [int]$Rect.Left
    y = [int]$Rect.Top
    width = $width
    height = $height
    center_x = [int]($Rect.Left + ($width / 2))
    center_y = [int]($Rect.Top + ($height / 2))
  }
}

function Find-DesktopWindow([string]$WindowTitle, [string]$ProcessName, [int]$WindowHandle, [string]$MatchMode) {
  $windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll([System.Windows.Automation.TreeScope]::Children, [System.Windows.Automation.Condition]::TrueCondition)
  foreach ($candidate in $windows) {
    try {
      $current = $candidate.Current
      $handle = [int]$current.NativeWindowHandle
      if ($handle -eq 0) {
        continue
      }
      $title = [string]$current.Name
      $procName = Get-WindowProcessName([int]$current.ProcessId)
      if ($WindowHandle -gt 0 -and $handle -ne $WindowHandle) {
        continue
      }
      if (-not (Match-DesktopValue $title $WindowTitle $MatchMode)) {
        continue
      }
      if (-not (Match-DesktopValue $procName $ProcessName "exact")) {
        continue
      }
      return $candidate
    } catch {
    }
  }
  return $null
}

function Get-ElementSnapshot($Element) {
  $current = $Element.Current
  $rect = $current.BoundingRectangle
  $bounds = Get-BoundsObject $rect
  [pscustomobject]@{
    name = [string]$current.Name
    automation_id = [string]$current.AutomationId
    class_name = [string]$current.ClassName
    control_type = (Get-ControlTypeName $current.ControlType)
    process_id = [int]$current.ProcessId
    handle = [int]$current.NativeWindowHandle
    is_enabled = [bool]$current.IsEnabled
    has_keyboard_focus = [bool]$current.HasKeyboardFocus
    is_offscreen = [bool]$current.IsOffscreen
    x = $bounds.x
    y = $bounds.y
    width = $bounds.width
    height = $bounds.height
    center_x = $bounds.center_x
    center_y = $bounds.center_y
  }
}

function Match-AutomationElement($Element, [string]$ElementName, [string]$AutomationId, [string]$ClassName, [string]$ControlType, [string]$MatchMode, [bool]$IncludeOffscreen, [bool]$IncludeDisabled) {
  $snapshot = Get-ElementSnapshot $Element
  if (-not $IncludeOffscreen -and ($snapshot.width -le 0 -or $snapshot.height -le 0 -or $snapshot.is_offscreen)) {
    return $false
  }
  if (-not $IncludeDisabled -and -not $snapshot.is_enabled) {
    return $false
  }
  if (-not (Match-DesktopValue $snapshot.name $ElementName $MatchMode)) {
    return $false
  }
  if (-not (Match-DesktopValue $snapshot.automation_id $AutomationId "exact")) {
    return $false
  }
  if (-not (Match-DesktopValue $snapshot.class_name $ClassName $MatchMode)) {
    return $false
  }
  if (-not (Match-DesktopValue $snapshot.control_type $ControlType "exact")) {
    return $false
  }
  return $true
}
`

func DesktopListWindowsTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_list_windows", opts, true); err != nil {
		return "", err
	}
	windows, err := runDesktopWindowQuery(ctx, input)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(windows)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopWaitWindowTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_wait_window", opts, true); err != nil {
		return "", err
	}
	timeoutMS, ok := numberInput(input["timeout_ms"])
	if !ok || timeoutMS <= 0 {
		timeoutMS = 10000
	}
	intervalMS, ok := numberInput(input["interval_ms"])
	if !ok || intervalMS <= 0 {
		intervalMS = 500
	}
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		attempts++
		windows, err := runDesktopWindowQuery(ctx, input)
		if err != nil {
			return "", err
		}
		if len(windows) > 0 {
			payload, err := json.Marshal(map[string]any{
				"attempts": attempts,
				"window":   windows[0],
			})
			if err != nil {
				return "", err
			}
			return string(payload), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("window did not appear within %dms", timeoutMS)
		}
		timer := time.NewTimer(time.Duration(intervalMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func DesktopInspectUITool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_inspect_ui", opts, true); err != nil {
		return "", err
	}
	selector, err := resolveDesktopAutomationSelector(input, true)
	if err != nil {
		return "", err
	}
	script := desktopAutomationPSScriptPrelude + fmt.Sprintf(`
$windowTitle = %s;
$processName = %s;
$windowHandle = %d;
$matchMode = %s;
$scopeName = %s;
$elementName = %s;
$automationId = %s;
$className = %s;
$controlType = %s;
$maxElements = %d;
$includeOffscreen = %s;
$includeDisabled = %s;

$targetWindow = Find-DesktopWindow $windowTitle $processName $windowHandle $matchMode
if (-not $targetWindow) {
  throw "window not found"
}
$scope = if ($scopeName -eq "children") { [System.Windows.Automation.TreeScope]::Children } else { [System.Windows.Automation.TreeScope]::Descendants }
$elements = $targetWindow.FindAll($scope, [System.Windows.Automation.Condition]::TrueCondition)
$items = @()
foreach ($candidate in $elements) {
  try {
    if (-not (Match-AutomationElement $candidate $elementName $automationId $className $controlType $matchMode $includeOffscreen $includeDisabled)) {
      continue
    }
    $items += Get-ElementSnapshot $candidate
    if ($items.Count -ge $maxElements) {
      break
    }
  } catch {
  }
}
$items | ConvertTo-Json -Depth 6 -Compress
`,
		powerShellString(selector.WindowTitle),
		powerShellString(selector.ProcessName),
		selector.Handle,
		powerShellString(selector.Match),
		powerShellString(selector.Scope),
		powerShellString(selector.Name),
		powerShellString(selector.AutomationID),
		powerShellString(selector.ClassName),
		powerShellString(selector.ControlType),
		selector.MaxElements,
		powerShellBool(selector.IncludeOffscreen),
		powerShellBool(selector.IncludeDisabled),
	)
	output, err := runDesktopPowerShell(ctx, script)
	if err != nil {
		return "", err
	}
	elements, err := unmarshalJSONObjectOrArray[desktopAutomationElement](output)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(elements)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopInvokeUITool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_invoke_ui", opts, false); err != nil {
		return "", err
	}
	selector, err := resolveDesktopAutomationSelector(input, true)
	if err != nil {
		return "", err
	}
	action := strings.TrimSpace(strings.ToLower(stringValue(input["action"])))
	if action == "" {
		action = "auto"
	}
	switch action {
	case "auto", "invoke", "click", "focus", "select", "expand", "collapse", "toggle":
	default:
		return "", fmt.Errorf("action must be auto, invoke, click, focus, select, expand, collapse, or toggle")
	}
	script := desktopAutomationPSScriptPrelude + fmt.Sprintf(`
$windowTitle = %s;
$processName = %s;
$windowHandle = %d;
$matchMode = %s;
$scopeName = %s;
$elementName = %s;
$automationId = %s;
$className = %s;
$controlType = %s;
$elementIndex = %d;
$requestedAction = %s;
$includeOffscreen = %s;
$includeDisabled = %s;

$targetWindow = Find-DesktopWindow $windowTitle $processName $windowHandle $matchMode
if (-not $targetWindow) {
  throw "window not found"
}
$scope = if ($scopeName -eq "children") { [System.Windows.Automation.TreeScope]::Children } else { [System.Windows.Automation.TreeScope]::Descendants }
$elements = $targetWindow.FindAll($scope, [System.Windows.Automation.Condition]::TrueCondition)
$matches = New-Object System.Collections.ArrayList
foreach ($candidate in $elements) {
  try {
    if (-not (Match-AutomationElement $candidate $elementName $automationId $className $controlType $matchMode $includeOffscreen $includeDisabled)) {
      continue
    }
    [void]$matches.Add($candidate)
  } catch {
  }
}
if ($matches.Count -lt $elementIndex) {
  throw "automation element not found"
}
$target = $matches[$elementIndex - 1]
$performedAction = ""
$target.SetFocus()
Start-Sleep -Milliseconds 50
$pattern = $null
if (($requestedAction -eq "invoke" -or $requestedAction -eq "auto") -and $target.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) {
  $pattern.Invoke()
  $performedAction = "invoke"
}
if (-not $performedAction -and ($requestedAction -eq "select" -or $requestedAction -eq "auto") -and $target.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern, [ref]$pattern)) {
  $pattern.Select()
  $performedAction = "select"
}
if (-not $performedAction -and ($requestedAction -eq "expand" -or $requestedAction -eq "auto") -and $target.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern, [ref]$pattern)) {
  $pattern.Expand()
  $performedAction = "expand"
}
if (-not $performedAction -and $requestedAction -eq "collapse" -and $target.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern, [ref]$pattern)) {
  $pattern.Collapse()
  $performedAction = "collapse"
}
if (-not $performedAction -and ($requestedAction -eq "toggle" -or $requestedAction -eq "auto") -and $target.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern, [ref]$pattern)) {
  $pattern.Toggle()
  $performedAction = "toggle"
}
if (-not $performedAction -and $requestedAction -eq "focus") {
  $performedAction = "focus"
}
if (-not $performedAction -and ($requestedAction -eq "click" -or $requestedAction -eq "auto")) {
  Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DesktopNative {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@;
  $snapshot = Get-ElementSnapshot $target
  if ($snapshot.width -le 0 -or $snapshot.height -le 0) {
    throw "automation element has no clickable bounds"
  }
  [DesktopNative]::SetCursorPos($snapshot.center_x, $snapshot.center_y) | Out-Null
  [DesktopNative]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
  [DesktopNative]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
  $performedAction = "click"
}
if (-not $performedAction) {
  throw "unable to invoke automation element"
}
[pscustomobject]@{
  action = $performedAction
  element = Get-ElementSnapshot $target
} | ConvertTo-Json -Depth 6 -Compress
`,
		powerShellString(selector.WindowTitle),
		powerShellString(selector.ProcessName),
		selector.Handle,
		powerShellString(selector.Match),
		powerShellString(selector.Scope),
		powerShellString(selector.Name),
		powerShellString(selector.AutomationID),
		powerShellString(selector.ClassName),
		powerShellString(selector.ControlType),
		selector.Index,
		powerShellString(action),
		powerShellBool(selector.IncludeOffscreen),
		powerShellBool(selector.IncludeDisabled),
	)
	output, err := runDesktopPowerShell(ctx, script)
	if err != nil {
		return "", err
	}
	result := desktopAutomationActionResult{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		return "", err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopSetValueUITool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_set_value_ui", opts, false); err != nil {
		return "", err
	}
	selector, err := resolveDesktopAutomationSelector(input, true)
	if err != nil {
		return "", err
	}
	value := stringValue(input["value"])
	if value == "" {
		return "", fmt.Errorf("value is required")
	}
	appendValue, _ := input["append"].(bool)
	submit, _ := input["submit"].(bool)
	script := desktopAutomationPSScriptPrelude + fmt.Sprintf(`
$windowTitle = %s;
$processName = %s;
$windowHandle = %d;
$matchMode = %s;
$scopeName = %s;
$elementName = %s;
$automationId = %s;
$className = %s;
$controlType = %s;
$elementIndex = %d;
$valueText = %s;
$appendValue = %s;
$submit = %s;
$includeOffscreen = %s;
$includeDisabled = %s;

$targetWindow = Find-DesktopWindow $windowTitle $processName $windowHandle $matchMode
if (-not $targetWindow) {
  throw "window not found"
}
$scope = if ($scopeName -eq "children") { [System.Windows.Automation.TreeScope]::Children } else { [System.Windows.Automation.TreeScope]::Descendants }
$elements = $targetWindow.FindAll($scope, [System.Windows.Automation.Condition]::TrueCondition)
$matches = New-Object System.Collections.ArrayList
foreach ($candidate in $elements) {
  try {
    if (-not (Match-AutomationElement $candidate $elementName $automationId $className $controlType $matchMode $includeOffscreen $includeDisabled)) {
      continue
    }
    [void]$matches.Add($candidate)
  } catch {
  }
}
if ($matches.Count -lt $elementIndex) {
  throw "automation element not found"
}
$target = $matches[$elementIndex - 1]
$target.SetFocus()
Start-Sleep -Milliseconds 50
$performedAction = ""
$pattern = $null
if ($target.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern, [ref]$pattern)) {
  if ($appendValue) {
    $pattern.SetValue(([string]$pattern.Current.Value) + $valueText)
  } else {
    $pattern.SetValue($valueText)
  }
  $performedAction = "value_pattern"
} else {
  Add-Type -AssemblyName System.Windows.Forms
  if (-not $appendValue) {
    [System.Windows.Forms.SendKeys]::SendWait("^a")
    Start-Sleep -Milliseconds 50
    [System.Windows.Forms.SendKeys]::SendWait("{BACKSPACE}")
    Start-Sleep -Milliseconds 50
  }
  [System.Windows.Forms.SendKeys]::SendWait(%s)
  $performedAction = "send_keys"
}
if ($submit) {
  Add-Type -AssemblyName System.Windows.Forms
  Start-Sleep -Milliseconds 50
  [System.Windows.Forms.SendKeys]::SendWait("{ENTER}")
}
[pscustomobject]@{
  action = $performedAction
  value = $valueText
  submit = $submit
  element = Get-ElementSnapshot $target
} | ConvertTo-Json -Depth 6 -Compress
`,
		powerShellString(selector.WindowTitle),
		powerShellString(selector.ProcessName),
		selector.Handle,
		powerShellString(selector.Match),
		powerShellString(selector.Scope),
		powerShellString(selector.Name),
		powerShellString(selector.AutomationID),
		powerShellString(selector.ClassName),
		powerShellString(selector.ControlType),
		selector.Index,
		powerShellString(value),
		powerShellBool(appendValue),
		powerShellBool(submit),
		powerShellBool(selector.IncludeOffscreen),
		powerShellBool(selector.IncludeDisabled),
		powerShellString(sendKeysEscape(value)),
	)
	output, err := runDesktopPowerShell(ctx, script)
	if err != nil {
		return "", err
	}
	result := desktopAutomationActionResult{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		return "", err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func runDesktopWindowQuery(ctx context.Context, input map[string]any) ([]desktopWindowInfo, error) {
	title := strings.TrimSpace(stringValue(input["title"]))
	processName := strings.TrimSpace(stringValue(input["process_name"]))
	handle, _ := numberInput(input["handle"])
	match := normalizeDesktopMatchMode(stringValue(input["match"]))
	activeOnly, _ := input["active_only"].(bool)
	script := desktopAutomationPSScriptPrelude + fmt.Sprintf(`
$windowTitle = %s;
$processName = %s;
$windowHandle = %d;
$matchMode = %s;
$activeOnly = %s;

$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll([System.Windows.Automation.TreeScope]::Children, [System.Windows.Automation.Condition]::TrueCondition)
$items = @()
foreach ($candidate in $windows) {
  try {
    $current = $candidate.Current
    $handle = [int]$current.NativeWindowHandle
    if ($handle -eq 0) {
      continue
    }
    $title = [string]$current.Name
    $procName = Get-WindowProcessName([int]$current.ProcessId)
    $focused = [bool]$current.HasKeyboardFocus
    if ($windowHandle -gt 0 -and $handle -ne $windowHandle) {
      continue
    }
    if (-not (Match-DesktopValue $title $windowTitle $matchMode)) {
      continue
    }
    if (-not (Match-DesktopValue $procName $processName "exact")) {
      continue
    }
    if ($activeOnly -and -not $focused) {
      continue
    }
    $rect = $current.BoundingRectangle
    $bounds = Get-BoundsObject $rect
    $items += [pscustomobject]@{
      title = $title
      process_name = $procName
      process_id = [int]$current.ProcessId
      handle = $handle
      x = $bounds.x
      y = $bounds.y
      width = $bounds.width
      height = $bounds.height
      center_x = $bounds.center_x
      center_y = $bounds.center_y
      is_focused = $focused
    }
  } catch {
  }
}
$items | ConvertTo-Json -Depth 5 -Compress
`,
		powerShellString(title),
		powerShellString(processName),
		handle,
		powerShellString(match),
		powerShellBool(activeOnly),
	)
	output, err := runDesktopPowerShell(ctx, script)
	if err != nil {
		return nil, err
	}
	return unmarshalJSONObjectOrArray[desktopWindowInfo](output)
}

func resolveDesktopAutomationSelector(input map[string]any, requireWindow bool) (desktopAutomationSelector, error) {
	selector := desktopAutomationSelector{
		WindowTitle:      strings.TrimSpace(stringValue(input["title"])),
		ProcessName:      strings.TrimSpace(stringValue(input["process_name"])),
		Match:            normalizeDesktopMatchMode(stringValue(input["match"])),
		Scope:            normalizeDesktopScope(stringValue(input["scope"])),
		Name:             strings.TrimSpace(stringValue(input["name"])),
		AutomationID:     strings.TrimSpace(stringValue(input["automation_id"])),
		ClassName:        strings.TrimSpace(stringValue(input["class_name"])),
		ControlType:      normalizeDesktopControlType(stringValue(input["control_type"])),
		IncludeOffscreen: boolValue(input["include_offscreen"]),
		IncludeDisabled:  boolValue(input["include_disabled"]),
	}
	handle, _ := numberInput(input["handle"])
	selector.Handle = handle
	index, ok := numberInput(input["index"])
	if !ok || index <= 0 {
		index = 1
	}
	selector.Index = index
	maxElements, ok := numberInput(input["max_elements"])
	if !ok || maxElements <= 0 {
		maxElements = 50
	}
	if maxElements > 200 {
		maxElements = 200
	}
	selector.MaxElements = maxElements
	if requireWindow && !desktopWindowSelectionProvided(selector) {
		return selector, fmt.Errorf("title, process_name, or handle is required")
	}
	if selector.Name == "" && selector.AutomationID == "" && selector.ClassName == "" && selector.ControlType == "" {
		if requireWindow {
			selector.MaxElements = maxElements
		}
	}
	return selector, nil
}

func desktopWindowSelectionProvided(selector desktopAutomationSelector) bool {
	return strings.TrimSpace(selector.WindowTitle) != "" || strings.TrimSpace(selector.ProcessName) != "" || selector.Handle > 0
}

func normalizeDesktopMatchMode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "exact":
		return "exact"
	default:
		return "contains"
	}
}

func normalizeDesktopScope(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "children":
		return "children"
	default:
		return "descendants"
	}
}

func normalizeDesktopControlType(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
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

func unmarshalJSONObjectOrArray[T any](raw string) ([]T, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var items []T
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		return items, nil
	}
	var item T
	if err := json.Unmarshal([]byte(raw), &item); err == nil {
		return []T{item}, nil
	}
	return nil, fmt.Errorf("failed to parse JSON output: %s", raw)
}
