package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const UILearnVersion = "anyclaw.app.ui-learn.v1"

type UILearnConfig struct {
	AppID       string `json:"app_id"`
	AppName     string `json:"app_name"`
	WindowTitle string `json:"window_title"`
	ProcessName string `json:"process_name"`
	AutoLearn   bool   `json:"auto_learn"`
	MaxElements int    `json:"max_elements"`
	Timeout     int    `json:"timeout_ms"`
}

type UISelector struct {
	Name         string       `json:"name,omitempty"`
	AutomationID string       `json:"automation_id,omitempty"`
	ClassName    string       `json:"class_name,omitempty"`
	ControlType  string       `json:"control_type,omitempty"`
	Index        int          `json:"index,omitempty"`
	Match        string       `json:"match,omitempty"`
	Fallbacks    []UISelector `json:"fallbacks,omitempty"`
}

type LearnedElement struct {
	ID          string     `json:"id"`
	Label       string     `json:"label"`
	Description string     `json:"description,omitempty"`
	Selector    UISelector `json:"selector"`
	Verified    bool       `json:"verified"`
	UsageCount  int        `json:"usage_count"`
	SuccessRate float64    `json:"success_rate"`
	LastUsed    time.Time  `json:"last_used,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type UIPairing struct {
	ID           string            `json:"id"`
	AppID        string            `json:"app_id"`
	AppName      string            `json:"app_name"`
	Workflow     string            `json:"workflow"`
	StepName     string            `json:"step_name"`
	Elements     []LearnedElement  `json:"elements"`
	InputMapping map[string]string `json:"input_mapping,omitempty"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type LearnSession struct {
	ID          string           `json:"id"`
	AppID       string           `json:"app_id"`
	AppName     string           `json:"app_name"`
	WindowTitle string           `json:"window_title"`
	Status      string           `json:"status"`
	Elements    []LearnedElement `json:"elements"`
	StepIndex   int              `json:"step_index"`
	StartedAt   time.Time        `json:"started_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

func NewUILearnConfig(appID, appName, windowTitle, processName string) *UILearnConfig {
	return &UILearnConfig{
		AppID:       appID,
		AppName:     appName,
		WindowTitle: windowTitle,
		ProcessName: processName,
		AutoLearn:   false,
		MaxElements: 100,
		Timeout:     30000,
	}
}

func StartLearnSession(ctx context.Context, config *UILearnConfig) (*LearnSession, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	window, err := probeWindowByProcess(ctx, config.ProcessName, config.WindowTitle)
	if err != nil {
		return nil, fmt.Errorf("app not running. please start %s first", config.AppName)
	}

	session := &LearnSession{
		ID:          fmt.Sprintf("learn_%d", time.Now().UnixNano()),
		AppID:       config.AppID,
		AppName:     config.AppName,
		WindowTitle: window.Title,
		Status:      "ready",
		StepIndex:   0,
		StartedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return session, nil
}

func (s *LearnSession) CaptureUI(ctx context.Context, maxElements int) ([]LearnedElement, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("only supported on Windows")
	}

	if maxElements <= 0 {
		maxElements = 100
	}

	app := &AppInfo{
		Name:        s.AppName,
		ProcessName: s.WindowTitle,
		WindowTitle: s.WindowTitle,
	}

	uiMap, err := InspectAppUI(ctx, app, maxElements)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect UI: %w", err)
	}

	elements := make([]LearnedElement, 0, len(uiMap.Elements))
	for i, el := range uiMap.Elements {
		if el.Name == "" && el.AutomationID == "" {
			continue
		}
		elem := LearnedElement{
			ID:    fmt.Sprintf("elem_%d", i),
			Label: el.Name,
			Selector: UISelector{
				Name:         el.Name,
				AutomationID: el.AutomationID,
				ClassName:    el.ClassName,
				ControlType:  el.ControlType,
				Index:        1,
				Match:        "contains",
			},
			Verified:   false,
			UsageCount: 0,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		elements = append(elements, elem)
	}

	s.Elements = elements
	s.UpdatedAt = time.Now()
	return elements, nil
}

func (s *LearnSession) VerifyElement(ctx context.Context, elementID string) (bool, error) {
	for i, elem := range s.Elements {
		if elem.ID == elementID {
			result, err := testSelector(ctx, s.WindowTitle, elem.Selector)
			if err != nil {
				return false, err
			}
			s.Elements[i].Verified = result
			s.Elements[i].UpdatedAt = time.Now()
			return result, nil
		}
	}
	return false, fmt.Errorf("element not found: %s", elementID)
}

func (s *LearnSession) AddFallback(elementID string, fallback UISelector) error {
	for i, elem := range s.Elements {
		if elem.ID == elementID {
			s.Elements[i].Selector.Fallbacks = append(s.Elements[i].Selector.Fallbacks, fallback)
			s.Elements[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("element not found: %s", elementID)
}

func (s *LearnSession) SavePairing(workflow, stepName string) *UIPairing {
	pairing := &UIPairing{
		ID:        fmt.Sprintf("pairing_%d", time.Now().UnixNano()),
		AppID:     s.AppID,
		AppName:   s.AppName,
		Workflow:  workflow,
		StepName:  stepName,
		Elements:  s.Elements,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return pairing
}

func testSelector(ctx context.Context, windowTitle string, selector UISelector) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, fmt.Errorf("only supported on Windows")
	}

	script := fmt.Sprintf(`
$windowTitle = '%s';
$elementName = '%s';
$automationId = '%s';
$controlType = '%s';

$window = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
	[System.Windows.Automation.TreeScope]::Children, 
	[System.Windows.Automation.Condition]::TrueCondition
) | Where-Object { $_.Current.Name -like "*$windowTitle*" } | Select-Object -First 1;

if (-not $window) { 
	@{ found = $false; error = "window not found" } | ConvertTo-Json -Compress
	exit
}

$elements = $window.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition);
$found = $false;
foreach ($e in $elements) {
	$current = $e.Current;
	if ($current.Name -like "*$elementName*" -or $current.AutomationId -eq "$automationId") {
		$found = $true;
		break;
	}
}
@{ found = $found } | ConvertTo-Json -Compress
`, windowTitle, selector.Name, selector.AutomationID, selector.ControlType)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to test selector: %w", err)
	}

	var result struct {
		Found bool `json:"found"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, fmt.Errorf("failed to parse result: %w", err)
	}

	return result.Found, nil
}

func probeWindowByProcess(ctx context.Context, processName, windowTitle string) (*AppWindowState, error) {
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
`, processName)

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

func RenderSelector(selector UISelector) map[string]any {
	result := map[string]any{
		"name": selector.Name,
	}
	if selector.AutomationID != "" {
		result["automation_id"] = selector.AutomationID
	}
	if selector.ClassName != "" {
		result["class_name"] = selector.ClassName
	}
	if selector.ControlType != "" {
		result["control_type"] = selector.ControlType
	}
	if selector.Index > 0 {
		result["index"] = selector.Index
	}
	if selector.Match != "" {
		result["match"] = selector.Match
	}
	return result
}

func ResolveSelectorWithFallback(ctx context.Context, windowTitle string, selector UISelector) (map[string]any, error) {
	primary := RenderSelector(selector)

	if runtime.GOOS == "windows" {
		found, err := testSelector(ctx, windowTitle, selector)
		if err == nil && found {
			return primary, nil
		}
	}

	for _, fallback := range selector.Fallbacks {
		if runtime.GOOS == "windows" {
			found, err := testSelector(ctx, windowTitle, fallback)
			if err == nil && found {
				return RenderSelector(fallback), nil
			}
		} else {
			return RenderSelector(fallback), nil
		}
	}

	return primary, fmt.Errorf("all selectors failed")
}

type PairingStore struct {
	Path     string
	Pairings []*UIPairing
}

func NewPairingStore(configPath string) (*PairingStore, error) {
	path, err := resolveStorePath(configPath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	store := &PairingStore{
		Path:     filepath.Join(dir, "ui-pairings.json"),
		Pairings: []*UIPairing{},
	}
	if err := store.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return store, nil
}

func (s *PairingStore) load() error {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.Pairings)
}

func (s *PairingStore) save() error {
	data, err := json.MarshalIndent(s.Pairings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o644)
}

func (s *PairingStore) List() []*UIPairing {
	return s.Pairings
}

func (s *PairingStore) Get(appID, workflow string) *UIPairing {
	for _, p := range s.Pairings {
		if p.AppID == appID && p.Workflow == workflow {
			return p
		}
	}
	return nil
}

func (s *PairingStore) Upsert(pairing *UIPairing) error {
	for i, p := range s.Pairings {
		if p.ID == pairing.ID {
			pairing.UpdatedAt = time.Now()
			s.Pairings[i] = pairing
			return s.save()
		}
	}
	s.Pairings = append(s.Pairings, pairing)
	return s.save()
}

func (s *PairingStore) Delete(id string) error {
	filtered := make([]*UIPairing, 0)
	for _, p := range s.Pairings {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == len(s.Pairings) {
		return fmt.Errorf("pairing not found")
	}
	s.Pairings = filtered
	return s.save()
}

func (s *PairingStore) RecordUsage(elementID string, success bool) {
	for _, pairing := range s.Pairings {
		for i, elem := range pairing.Elements {
			if elem.ID == elementID {
				elem.UsageCount++
				if success {
					elem.SuccessRate = (elem.SuccessRate*float64(elem.UsageCount-1) + 1) / float64(elem.UsageCount)
				} else {
					elem.SuccessRate = (elem.SuccessRate * float64(elem.UsageCount-1)) / float64(elem.UsageCount)
				}
				elem.LastUsed = time.Now()
				pairing.Elements[i] = elem
			}
		}
	}
	s.save()
}

func GeneratePairingJSON(pairing *UIPairing) string {
	data, _ := json.MarshalIndent(pairing, "", "  ")
	return string(data)
}
