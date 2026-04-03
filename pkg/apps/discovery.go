package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const AppDiscoveryVersion = "anyclaw.app.discovery.v1"

type AppInfo struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Path         string         `json:"path"`
	ProcessName  string         `json:"process_name"`
	WindowTitle  string         `json:"window_title"`
	Icon         string         `json:"icon,omitempty"`
	Category     string         `json:"category"`
	Vendor       string         `json:"vendor,omitempty"`
	Version      string         `json:"version,omitempty"`
	Args         string         `json:"args,omitempty"`
	Enabled      bool           `json:"enabled"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	DiscoveredAt time.Time      `json:"discovered_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type AppWindowState struct {
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	ProcessID   int    `json:"process_id"`
	Handle      int    `json:"handle"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	IsFocused   bool   `json:"is_focused"`
}

type UIElement struct {
	Name         string `json:"name"`
	AutomationID string `json:"automation_id"`
	ClassName    string `json:"class_name"`
	ControlType  string `json:"control_type"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	CenterX      int    `json:"center_x"`
	CenterY      int    `json:"center_y"`
	IsEnabled    bool   `json:"is_enabled"`
}

type UIMap struct {
	AppID       string      `json:"app_id"`
	AppName     string      `json:"app_name"`
	GeneratedAt time.Time   `json:"generated_at"`
	WindowTitle string      `json:"window_title"`
	Elements    []UIElement `json:"elements"`
}

type DiscoveryConfig struct {
	ScanPaths   []string   `json:"scan_paths"`
	KnownApps   []KnownApp `json:"known_apps"`
	AutoLaunch  bool       `json:"auto_launch"`
	AutoInspect bool       `json:"auto_inspect"`
	MaxElements int        `json:"max_elements"`
	Timeout     int        `json:"timeout_ms"`
}

type KnownApp struct {
	Name           string   `json:"name"`
	ProcessName    string   `json:"process_name"`
	WindowTitle    string   `json:"window_title"`
	Executable     string   `json:"executable"`
	Args           string   `json:"args"`
	Category       string   `json:"category"`
	Vendor         string   `json:"vendor"`
	DetectionPaths []string `json:"detection_paths"`
}

var DefaultKnownApps = []KnownApp{
	{
		Name: "QQ", ProcessName: "QQ", WindowTitle: "QQ",
		Executable: "QQ.exe", Category: "IM", Vendor: "Tencent",
		DetectionPaths: []string{
			"%ProgramFiles%/Tencent/QQ/QQ.exe",
			"%ProgramFiles(x86)%/Tencent/QQ/QQ.exe",
			"%LocalAppData%/Programs/QQ/QQ.exe",
		},
	},
	{
		Name: "WeChat", ProcessName: "WeChat", WindowTitle: "微信",
		Executable: "WeChat.exe", Category: "IM", Vendor: "Tencent",
		DetectionPaths: []string{
			"%ProgramFiles%/Tencent/WeChat/WeChat.exe",
			"%ProgramFiles(x86)%/Tencent/WeChat/WeChat.exe",
		},
	},
	{
		Name: "DingTalk", ProcessName: "DingTalk", WindowTitle: "钉钉",
		Executable: "DingTalk.exe", Category: "IM", Vendor: "Alibaba",
		DetectionPaths: []string{
			"%ProgramFiles%/DingDing/DingTalk.exe",
			"%ProgramFiles(x86)%/DingDing/DingTalk.exe",
			"%AppData%/Roaming/DingTalk/DingTalk.exe",
		},
	},
	{
		Name: "Feishu", ProcessName: "feishu", WindowTitle: "飞书",
		Executable: "feishu.exe", Category: "IM", Vendor: "ByteDance",
		DetectionPaths: []string{
			"%ProgramFiles%/Bytedance/FeiShu/FeiShu.exe",
			"%ProgramFiles(x86)%/Bytedance/FeiShu/FeiShu.exe",
		},
	},
	{
		Name: "Notepad", ProcessName: "notepad", WindowTitle: "记事本",
		Executable: "notepad.exe", Category: "Editor", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%SystemRoot%/System32/notepad.exe",
			"%SystemRoot%/SysWOW64/notepad.exe",
		},
	},
	{
		Name: "Notepad++", ProcessName: "notepad++", WindowTitle: "Notepad++",
		Executable: "notepad++.exe", Category: "Editor", Vendor: "Notepad++ Team",
		DetectionPaths: []string{
			"%ProgramFiles%/Notepad++/notepad++.exe",
			"%ProgramFiles(x86)%/Notepad++/notepad++.exe",
		},
	},
	{
		Name: "VSCode", ProcessName: "Code", WindowTitle: "Visual Studio Code",
		Executable: "Code.exe", Category: "Editor", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%LocalAppData%/Programs/Microsoft VS Code/Code.exe",
			"%ProgramFiles%/Microsoft VS Code/Code.exe",
		},
	},
	{
		Name: "Chrome", ProcessName: "chrome", WindowTitle: "Google Chrome",
		Executable: "chrome.exe", Category: "Browser", Vendor: "Google",
		DetectionPaths: []string{
			"%ProgramFiles%/Google/Chrome/Application/chrome.exe",
			"%ProgramFiles(x86)%/Google/Chrome/Application/chrome.exe",
		},
	},
	{
		Name: "Edge", ProcessName: "msedge", WindowTitle: "Microsoft Edge",
		Executable: "msedge.exe", Category: "Browser", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%ProgramFiles%/Microsoft/Edge/Application/msedge.exe",
			"%ProgramFiles(x86)%/Microsoft/Edge/Application/msedge.exe",
		},
	},
	{
		Name: "Explorer", ProcessName: "explorer", WindowTitle: "",
		Executable: "explorer.exe", Category: "System", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%SystemRoot%/explorer.exe",
		},
	},
	{
		Name: "Word", ProcessName: "WINWORD", WindowTitle: "Word",
		Executable: "WINWORD.EXE", Category: "Office", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%ProgramFiles%/Microsoft Office/root/Office*/WINWORD.EXE",
			"%ProgramFiles(x86)%/Microsoft Office/root/Office*/WINWORD.EXE",
		},
	},
	{
		Name: "Excel", ProcessName: "EXCEL", WindowTitle: "Excel",
		Executable: "EXCEL.EXE", Category: "Office", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%ProgramFiles%/Microsoft Office/root/Office*/EXCEL.EXE",
			"%ProgramFiles(x86)%/Microsoft Office/root/Office*/EXCEL.EXE",
		},
	},
	{
		Name: "PowerPoint", ProcessName: "POWERPNT", WindowTitle: "PowerPoint",
		Executable: "POWERPNT.EXE", Category: "Office", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%ProgramFiles%/Microsoft Office/root/Office*/POWERPNT.EXE",
			"%ProgramFiles(x86)%/Microsoft Office/root/Office*/POWERPNT.EXE",
		},
	},
	{
		Name: "WPS", ProcessName: "wps", WindowTitle: "WPS",
		Executable: "wps.exe", Category: "Office", Vendor: "Kingsoft",
		DetectionPaths: []string{
			"%ProgramFiles%/Kingsoft/WPS Office/11.2.0.XXXX/wps.exe",
			"%ProgramFiles(x86)%/Kingsoft/WPS Office/11.2.0.XXXX/wps.exe",
		},
	},
	{
		Name: "Teams", ProcessName: "Teams", WindowTitle: "Microsoft Teams",
		Executable: "Teams.exe", Category: "IM", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%LocalAppData%/Microsoft/Teams/Update.exe",
		},
	},
	{
		Name: "Slack", ProcessName: "slack", WindowTitle: "Slack",
		Executable: "slack.exe", Category: "IM", Vendor: "Slack",
		DetectionPaths: []string{
			"%LocalAppData%/slack/slack.exe",
			"%ProgramFiles%/Slack/Slack.exe",
		},
	},
	{
		Name: "Spotify", ProcessName: "Spotify", WindowTitle: "Spotify",
		Executable: "Spotify.exe", Category: "Media", Vendor: "Spotify",
		DetectionPaths: []string{
			"%LocalAppData%/Spotify/Spotify.exe",
			"%AppData%/Spotify/Spotify.exe",
		},
	},
	{
		Name: "Windows Terminal", ProcessName: "WindowsTerminal", WindowTitle: "Windows Terminal",
		Executable: "WindowsTerminal.exe", Category: "System", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%LocalAppData%/Microsoft/WindowsTerminal/WindowsTerminal.exe",
		},
	},
	{
		Name: "PowerShell", ProcessName: "powershell", WindowTitle: "PowerShell",
		Executable: "powershell.exe", Category: "System", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe",
			"%SystemRoot%/System32/WindowsPowerShell/v1.0/pwsh.exe",
		},
	},
	{
		Name: "Cmd", ProcessName: "cmd", WindowTitle: "命令提示符",
		Executable: "cmd.exe", Category: "System", Vendor: "Microsoft",
		DetectionPaths: []string{
			"%SystemRoot%/System32/cmd.exe",
		},
	},
}

func NewDiscoveryConfig() *DiscoveryConfig {
	return &DiscoveryConfig{
		ScanPaths:   getDefaultScanPaths(),
		KnownApps:   DefaultKnownApps,
		AutoLaunch:  false,
		AutoInspect: false,
		MaxElements: 100,
		Timeout:     30000,
	}
}

func getDefaultScanPaths() []string {
	paths := []string{}

	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	localAppData := os.Getenv("LocalAppData")
	appData := os.Getenv("AppData")
	systemRoot := os.Getenv("SystemRoot")

	if programFiles != "" {
		paths = append(paths, programFiles)
	}
	if programFilesX86 != "" && programFilesX86 != programFiles {
		paths = append(paths, programFilesX86)
	}
	if localAppData != "" {
		paths = append(paths, filepath.Join(localAppData, "Programs"))
		paths = append(paths, filepath.Join(localAppData, "Microsoft"))
	}
	if appData != "" {
		paths = append(paths, filepath.Join(appData, "Microsoft"))
		paths = append(paths, filepath.Join(appData, "Tencent"))
		paths = append(paths, filepath.Join(appData, "Bytedance"))
		paths = append(paths, filepath.Join(appData, "DingTalk"))
		paths = append(paths, filepath.Join(appData, "slack"))
		paths = append(paths, filepath.Join(appData, "Spotify"))
	}
	if systemRoot != "" {
		paths = append(paths, filepath.Join(systemRoot, "System32"))
	}

	return paths
}

func expandEnvPath(path string) string {
	expanded := os.ExpandEnv(path)
	expanded = strings.ReplaceAll(expanded, "XXXX", "*")
	return expanded
}

func findExecutable(searchPaths []string) string {
	for _, sp := range searchPaths {
		pattern := expandEnvPath(sp)
		if strings.Contains(pattern, "*") {
			matches, _ := filepath.Glob(pattern)
			for _, match := range matches {
				if _, err := os.Stat(match); err == nil {
					return match
				}
			}
		} else {
			if _, err := os.Stat(pattern); err == nil {
				return pattern
			}
		}
	}
	return ""
}

func DiscoverApps(ctx context.Context, config *DiscoveryConfig) ([]AppInfo, error) {
	if config == nil {
		config = NewDiscoveryConfig()
	}

	var apps []AppInfo

	for _, known := range config.KnownApps {
		select {
		case <-ctx.Done():
			return apps, ctx.Err()
		default:
		}

		path := findExecutable(known.DetectionPaths)
		if path == "" {
			continue
		}

		app := AppInfo{
			ID:          sanitizeID(known.Name),
			Name:        known.Name,
			Path:        path,
			ProcessName: known.ProcessName,
			WindowTitle: known.WindowTitle,
			Category:    known.Category,
			Vendor:      known.Vendor,
			Args:        known.Args,
			Enabled:     true,
			Metadata: map[string]any{
				"detection_source": "known_apps",
			},
			DiscoveredAt: time.Now(),
			UpdatedAt:    time.Now(),
		}

		if runtime.GOOS == "windows" {
			version, err := getFileVersion(path)
			if err == nil {
				app.Version = version
			}
		}

		apps = append(apps, app)
	}

	return apps, nil
}

func sanitizeID(name string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "+", "")
	id = strings.ReplaceAll(id, ".", "")
	return id
}

func getFileVersion(path string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("not supported on this platform")
	}

	cmd := exec.Command("powershell", "-Command",
		"(Get-Item '"+path+"').VersionInfo.FileVersion")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "unknown", nil
	}
	return version, nil
}

func ProbeAppWindow(ctx context.Context, app *AppInfo) (*AppWindowState, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("not supported on this platform")
	}

	script := fmt.Sprintf(`
$processName = '%s';
$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
);
foreach ($w in $windows) {
	$current = $w.Current;
	$handle = [int]$current.NativeWindowHandle;
	if ($handle -eq 0) { continue; }
	$proc = Get-Process -Id $current.ProcessId -ErrorAction SilentlyContinue;
	if ($proc -and $proc.ProcessName -eq $processName) {
		$rect = $current.BoundingRectangle;
		[PSCustomObject]@{
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
`, app.ProcessName)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to probe window: %w", err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, fmt.Errorf("window not found")
	}

	var window AppWindowState
	if err := json.Unmarshal([]byte(trimmed), &window); err != nil {
		return nil, fmt.Errorf("failed to parse window state: %w", err)
	}

	return &window, nil
}

func InspectAppUI(ctx context.Context, app *AppInfo, maxElements int) (*UIMap, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("not supported on this platform")
	}

	if maxElements <= 0 {
		maxElements = 100
	}

	window, err := ProbeAppWindow(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to find window: %w", err)
	}

	script := fmt.Sprintf(`
$handle = %d;
$maxElements = %d;

Add-Type -AssemblyName UIAutomationClient;
Add-Type -AssemblyName UIAutomationTypes;

function Get-Bounds($rect) {
	@{
		x = [int]$rect.Left;
		y = [int]$rect.Top;
		width = [int]($rect.Right - $rect.Left);
		height = [int]($rect.Bottom - $rect.Top);
	}
}

$window = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
) | Where-Object { [int]$_.Current.NativeWindowHandle -eq $handle } | Select-Object -First 1;

if (-not $window) { throw "Window not found" }

$elements = $window.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition);
$items = @();

foreach ($e in $elements) {
	try {
		$current = $e.Current;
		$rect = $current.BoundingRectangle;
		if ($rect.Width -le 0 -or $rect.Height -le 0) { continue; }
		
		$ctrlType = ([string]$current.ControlType.ProgrammaticName) -replace '^ControlType\\.', '';
		
		$items += @{
			name = [string]$current.Name;
			automation_id = [string]$current.AutomationId;
			class_name = [string]$current.ClassName;
			control_type = $ctrlType;
			x = [int]$rect.Left;
			y = [int]$rect.Top;
			width = [int]$rect.Width;
			height = [int]$rect.Height;
			center_x = [int]($rect.Left + $rect.Width / 2);
			center_y = [int]($rect.Top + $rect.Height / 2);
			is_enabled = [bool]$current.IsEnabled;
		};
		
		if ($items.Count -ge $maxElements) { break; }
	} catch { }
}

$items | ConvertTo-Json -Depth 3 -Compress
`, window.Handle, maxElements)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect UI: %w", err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		return nil, fmt.Errorf("no UI elements found")
	}

	var elements []UIElement
	if err := json.Unmarshal([]byte(trimmed), &elements); err != nil {
		var single UIElement
		if err2 := json.Unmarshal([]byte(trimmed), &single); err2 == nil {
			elements = []UIElement{single}
		} else {
			return nil, fmt.Errorf("failed to parse UI elements: %w", err)
		}
	}

	return &UIMap{
		AppID:       app.ID,
		AppName:     app.Name,
		GeneratedAt: time.Now(),
		WindowTitle: window.Title,
		Elements:    elements,
	}, nil
}

func ListRunningApps(ctx context.Context) ([]AppWindowState, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("not supported on this platform")
	}

	script := `
$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
);
$items = @();

foreach ($w in $windows) {
	$current = $w.Current;
	$handle = [int]$current.NativeWindowHandle;
	if ($handle -eq 0) { continue; }
	
	$proc = Get-Process -Id $current.ProcessId -ErrorAction SilentlyContinue;
	if (-not $proc) { continue; }
	
	$rect = $current.BoundingRectangle;
	$items += @{
		title = $current.Name;
		process_name = $proc.ProcessName;
		process_id = $current.ProcessId;
		handle = $handle;
		x = [int]$rect.Left;
		y = [int]$rect.Top;
		width = [int]($rect.Right - $rect.Left);
		height = [int]($rect.Bottom - $rect.Top);
		is_focused = [bool]$current.HasKeyboardFocus;
	};
}

$items | ConvertTo-Json -Depth 3 -Compress
`

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		return []AppWindowState{}, nil
	}

	var windows []AppWindowState
	if err := json.Unmarshal([]byte(trimmed), &windows); err != nil {
		var single AppWindowState
		if err2 := json.Unmarshal([]byte(trimmed), &single); err2 == nil {
			windows = []AppWindowState{single}
		} else {
			return nil, fmt.Errorf("failed to parse windows: %w", err)
		}
	}

	return windows, nil
}
