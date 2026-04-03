package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const WindowMonitorVersion = "anyclaw.app.window-monitor.v1"

type WindowStatus string

const (
	WindowStatusUnknown    WindowStatus = "unknown"
	WindowStatusNotRun     WindowStatus = "not_running"
	WindowStatusRunning    WindowStatus = "running"
	WindowStatusReady      WindowStatus = "ready"
	WindowStatusFocused    WindowStatus = "focused"
	WindowStatusMinimized  WindowStatus = "minimized"
	WindowStatusMaximized  WindowStatus = "maximized"
	WindowStatusBackground WindowStatus = "background"
)

type WindowProbeResult struct {
	Status       WindowStatus `json:"status"`
	ProcessName  string       `json:"process_name"`
	ProcessID    int          `json:"process_id"`
	WindowTitle  string       `json:"window_title"`
	WindowHandle int          `json:"window_handle"`
	X            int          `json:"x"`
	Y            int          `json:"y"`
	Width        int          `json:"width"`
	Height       int          `json:"height"`
	CenterX      int          `json:"center_x"`
	CenterY      int          `json:"center_y"`
	IsFocused    bool         `json:"is_focused"`
	IsMinimized  bool         `json:"is_minimized"`
	IsMaximized  bool         `json:"is_maximized"`
	IsVisible    bool         `json:"is_visible"`
	IsEnabled    bool         `json:"is_enabled"`
	ProbeTime    time.Time    `json:"probe_time"`
	Error        string       `json:"error,omitempty"`
}

type ExecutionProgress struct {
	PlanID         string              `json:"plan_id"`
	AppID          string              `json:"app_id"`
	CurrentStep    int                 `json:"current_step"`
	TotalSteps     int                 `json:"total_steps"`
	CompletedSteps []int               `json:"completed_steps"`
	FailedSteps    []int               `json:"failed_steps"`
	State          string              `json:"state"`
	LastStepTime   time.Time           `json:"last_step_time"`
	Checkpoint     *ProgressCheckpoint `json:"checkpoint,omitempty"`
}

type ProgressCheckpoint struct {
	StepIndex   int                `json:"step_index"`
	Label       string             `json:"label"`
	Timestamp   time.Time          `json:"timestamp"`
	WindowState *WindowProbeResult `json:"window_state,omitempty"`
	Data        map[string]any     `json:"data,omitempty"`
}

type WindowMonitor struct {
	mu            sync.RWMutex
	probes        map[string]*WindowProbeResult
	progress      map[string]*ExecutionProgress
	checkpointTTL time.Duration
}

func NewWindowMonitor() *WindowMonitor {
	return &WindowMonitor{
		probes:        make(map[string]*WindowProbeResult),
		progress:      make(map[string]*ExecutionProgress),
		checkpointTTL: 30 * time.Minute,
	}
}

func (m *WindowMonitor) Probe(processName, windowTitle string) (*WindowProbeResult, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := &WindowProbeResult{
		ProbeTime:   time.Now(),
		ProcessName: processName,
	}

	script := fmt.Sprintf(`
$procName = '%s';
$wantTitle = '%s';

$proc = Get-Process -Name $procName -ErrorAction SilentlyContinue | Select-Object -First 1;
if (-not $proc) {
	@{ status = "not_running"; process_name = $procName } | ConvertTo-Json -Compress;
	exit;
}

$result = @{
	status = "running";
	process_name = $procName;
	process_id = $proc.Id;
}

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
	[DllImport("user32.dll")] public static extern int GetWindowText(IntPtr hWnd, System.Text.StringBuilder text, int count);
	[DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
	[DllImport("user32.dll")] public static extern bool IsWindowEnabled(IntPtr hWnd);
	[DllImport("user32.dll")] public static extern int GetWindowLong(IntPtr hWnd, int nIndex);
	[DllImport("user32.dll")] public static extern IntPtr GetShellWindow();
}
"@

$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
);

$targetWin = $null;
$foreground = [Win32]::GetForegroundWindow();

foreach ($w in $windows) {
	$current = $w.Current;
	$handle = [int]$current.NativeWindowHandle;
	if ($handle -eq 0) { continue; }
	$proc = Get-Process -Id $current.ProcessId -ErrorAction SilentlyContinue;
	if ($proc -and $proc.ProcessName -eq $procName) {
		$targetWin = $w;
		$rect = $current.BoundingRectangle;
		
		$isMinimized = $current.IsOffscreen -or $rect.Width -eq 0 -or $rect.Height -eq 0;
		$isFocused = $handle -eq [IntPtr]$foreground;
		
		$result.window_handle = $handle;
		$result.title = $current.Name;
		$result.x = [int]$rect.Left;
		$result.y = [int]$rect.Top;
		$result.width = [int]($rect.Right - $rect.Left);
		$result.height = [int]($rect.Bottom - $rect.Top);
		$result.center_x = [int]($rect.Left + ($rect.Right - $rect.Left) / 2);
		$result.center_y = [int]($rect.Top + ($rect.Bottom - $rect.Top) / 2);
		$result.is_focused = $isFocused;
		$result.is_minimized = $isMinimized;
		$result.is_visible = [Win32]::IsWindowVisible([IntPtr]$handle);
		$result.is_enabled = [Win32]::IsWindowEnabled([IntPtr]$handle);
		
		if ($isFocused) {
			$result.status = "focused";
		} elseif ($isMinimized) {
			$result.status = "minimized";
		} elseif ($result.is_visible) {
			$result.status = "ready";
		} else {
			$result.status = "background";
		}
		
		break;
	}
}

if (-not $targetWin) {
	$result.status = "running_no_window";
}

$result | ConvertTo-Json -Compress;
`, processName, windowTitle)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		result.Status = WindowStatusUnknown
		result.Error = err.Error()
		return result, err
	}

	var probe WindowProbeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		result.Error = fmt.Sprintf("parse error: %s", err)
		return result, err
	}

	result = &probe
	result.ProbeTime = time.Now()

	m.mu.Lock()
	m.probes[processName] = result
	m.mu.Unlock()

	return result, nil
}

func (m *WindowMonitor) IsRunning(processName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probe, ok := m.probes[processName]
	if !ok {
		return false
	}

	return probe.Status == WindowStatusRunning ||
		probe.Status == WindowStatusReady ||
		probe.Status == WindowStatusFocused ||
		probe.Status == WindowStatusMinimized ||
		probe.Status == WindowStatusMaximized ||
		probe.Status == WindowStatusBackground
}

func (m *WindowMonitor) IsWindowReady(processName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probe, ok := m.probes[processName]
	if !ok {
		return false
	}

	return probe.Status == WindowStatusReady ||
		probe.Status == WindowStatusFocused
}

func (m *WindowMonitor) IsFocused(processName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probe, ok := m.probes[processName]
	if !ok {
		return false
	}

	return probe.Status == WindowStatusFocused
}

func (m *WindowMonitor) GetLastProbe(processName string) *WindowProbeResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.probes[processName]
}

func (m *WindowMonitor) GetAllProbes() map[string]*WindowProbeResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*WindowProbeResult)
	for k, v := range m.probes {
		result[k] = v
	}
	return result
}

func (m *WindowMonitor) StartProgress(planID, appID string, totalSteps int) *ExecutionProgress {
	m.mu.Lock()
	defer m.mu.Unlock()

	progress := &ExecutionProgress{
		PlanID:         planID,
		AppID:          appID,
		CurrentStep:    0,
		TotalSteps:     totalSteps,
		CompletedSteps: []int{},
		FailedSteps:    []int{},
		State:          "running",
		LastStepTime:   time.Now(),
	}

	m.progress[planID] = progress
	return progress
}

func (m *WindowMonitor) UpdateProgress(planID string, stepIndex int, windowState *WindowProbeResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	progress, ok := m.progress[planID]
	if !ok {
		return
	}

	progress.CurrentStep = stepIndex
	progress.LastStepTime = time.Now()

	if stepIndex > 0 && len(progress.CompletedSteps) < stepIndex {
		progress.CompletedSteps = append(progress.CompletedSteps, stepIndex)
	}

	if windowState != nil {
		progress.Checkpoint = &ProgressCheckpoint{
			StepIndex:   stepIndex,
			Timestamp:   time.Now(),
			WindowState: windowState,
		}
	}
}

func (m *WindowMonitor) MarkStepFailed(planID string, stepIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	progress, ok := m.progress[planID]
	if !ok {
		return
	}

	progress.FailedSteps = append(progress.FailedSteps, stepIndex)
}

func (m *WindowMonitor) CompleteProgress(planID string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	progress, ok := m.progress[planID]
	if !ok {
		return
	}

	if success {
		progress.State = "completed"
	} else {
		progress.State = "failed"
	}
}

func (m *WindowMonitor) GetProgress(planID string) *ExecutionProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.progress[planID]
}

func (m *WindowMonitor) GetLastCheckpoint(planID string) *ProgressCheckpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	progress, ok := m.progress[planID]
	if !ok {
		return nil
	}

	return progress.Checkpoint
}

func (m *WindowMonitor) ResumeFromCheckpoint(planID string) (int, error) {
	m.mu.RLock()
	progress, ok := m.progress[planID]
	m.mu.RUnlock()

	if !ok {
		return 0, fmt.Errorf("no progress found for plan: %s", planID)
	}

	if progress.Checkpoint == nil {
		return 0, nil
	}

	if progress.Checkpoint.WindowState != nil {
		windowState := progress.Checkpoint.WindowState

		if time.Since(windowState.ProbeTime) > m.checkpointTTL {
			return 0, fmt.Errorf("checkpoint expired, need to restart")
		}

		current, err := m.Probe(windowState.ProcessName, windowState.WindowTitle)
		if err != nil {
			return 0, fmt.Errorf("failed to probe window: %w", err)
		}

		if current.Status == WindowStatusNotRun {
			return 0, fmt.Errorf("app no longer running")
		}

		if current.Status == WindowStatusMinimized {
			script := fmt.Sprintf(`
$handle = %d;
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
	[DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
}
"@
[Win32]::ShowWindow([IntPtr]::%d, 9) | Out-Null;
`, current.WindowHandle, current.WindowHandle)
			exec.Command("powershell", "-Command", script).Run()
			time.Sleep(500 * time.Millisecond)
		}
	}

	return progress.Checkpoint.StepIndex, nil
}

func (m *WindowMonitor) ClearProgress(planID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.progress, planID)
}

func (m *WindowMonitor) ListProgress() []*ExecutionProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ExecutionProgress, 0, len(m.progress))
	for _, p := range m.progress {
		result = append(result, p)
	}

	return result
}

func (m *WindowMonitor) CleanupOldCheckpoints(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, p := range m.progress {
		if p.LastStepTime.Before(cutoff) {
			delete(m.progress, id)
		}
	}
}

type WindowStateChange struct {
	ProcessName string
	FromStatus  WindowStatus
	ToStatus    WindowStatus
	Timestamp   time.Time
}

func (m *WindowMonitor) WatchForChanges(processName string, interval time.Duration, callback func(WindowStateChange)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastStatus WindowStatus

	for {
		select {
		case <-ticker.C:
			result, err := m.Probe(processName, "")
			if err != nil {
				continue
			}

			if result.Status != lastStatus && lastStatus != "" {
				callback(WindowStateChange{
					ProcessName: processName,
					FromStatus:  lastStatus,
					ToStatus:    result.Status,
					Timestamp:   time.Now(),
				})
			}
			lastStatus = result.Status
		}
	}
}

func (m *WindowMonitor) WaitForWindow(processName, windowTitle string, timeout time.Duration) (*WindowProbeResult, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			result, err := m.Probe(processName, windowTitle)
			if err == nil {
				if result.Status == WindowStatusReady ||
					result.Status == WindowStatusFocused {
					return result, nil
				}
			}

			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for window")
			}
		}
	}
}

func (m *WindowMonitor) WaitForFocus(processName string, timeout time.Duration) (*WindowProbeResult, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			result, err := m.Probe(processName, "")
			if err == nil && result.Status == WindowStatusFocused {
				return result, nil
			}

			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for focus")
			}
		}
	}
}
