package verification

import (
	"context"
	"fmt"
	"time"

	"github.com/anyclaw/anyclaw/pkg/apps"
)

type ToolExecutor func(ctx context.Context, toolName string, input map[string]any) (string, error)

type IntegrationExecutor struct {
	executor *VerificationExecutor
	toolExec ToolExecutor
	registry *TemplateRegistry
}

func NewIntegrationExecutor(toolExec ToolExecutor) *IntegrationExecutor {
	exec := &IntegrationExecutor{
		toolExec: toolExec,
		registry: NewTemplateRegistry(),
	}
	RegisterDefaultTemplates(exec.registry)
	exec.executor = &VerificationExecutor{
		registry: exec.registry,
	}
	exec.initContext()
	return exec
}

func (ie *IntegrationExecutor) initContext() {
	ie.executor.context = &IntegrationContext{
		toolExec: ie.toolExec,
	}
}

type IntegrationContext struct {
	toolExec ToolExecutor
}

func (ic *IntegrationContext) FileExists(path string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "file_exists", map[string]any{"path": path})
	if err != nil {
		return false, err
	}
	return result == "true" || result == "exists", nil
}

func (ic *IntegrationContext) FileContains(path string, content string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "read_file", map[string]any{"path": path})
	if err != nil {
		return false, err
	}
	return contains(result, content), nil
}

func (ic *IntegrationContext) FileMatches(path string, pattern string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "read_file", map[string]any{"path": path})
	if err != nil {
		return false, err
	}
	return match(pattern, result), nil
}

func (ic *IntegrationContext) WindowAppears(title string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "desktop_list_windows", nil)
	if err != nil {
		return false, err
	}
	return contains(result, title), nil
}

func (ic *IntegrationContext) WindowFocused(title string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "desktop_get_foreground_window", nil)
	if err != nil {
		return false, err
	}
	return contains(result, title), nil
}

func (ic *IntegrationContext) TextContains(x, y, width, height int, text string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "desktop_ocr", map[string]any{
		"x": x, "y": y, "width": width, "height": height,
	})
	if err != nil {
		return false, err
	}
	return contains(result, text), nil
}

func (ic *IntegrationContext) OCRText(x, y, width, height int) (string, error) {
	return ic.toolExec(context.Background(), "desktop_ocr", map[string]any{
		"x": x, "y": y, "width": width, "height": height,
	})
}

func (ic *IntegrationContext) Clipboard() (string, error) {
	return ic.toolExec(context.Background(), "clipboard_read", nil)
}

func (ic *IntegrationContext) ClipboardContains(content string) (bool, bool) {
	result, err := ic.Clipboard()
	if err != nil || result == "" {
		return false, false
	}
	return contains(result, content), true
}

func (ic *IntegrationContext) NetworkRequest(url string) (int, error) {
	result, err := ic.toolExec(context.Background(), "http_request", map[string]any{
		"url": url, "method": "GET",
	})
	if err != nil {
		return 0, err
	}
	if contains(result, "200") || contains(result, "success") {
		return 200, nil
	}
	if contains(result, "404") {
		return 404, nil
	}
	if contains(result, "500") {
		return 500, nil
	}
	return 0, fmt.Errorf("unknown status: %s", result)
}

func (ic *IntegrationContext) AppRunning(appName string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "desktop_list_windows", nil)
	if err != nil {
		return false, err
	}
	return contains(result, appName), nil
}

func (ic *IntegrationContext) AppState(appName string) (map[string]any, error) {
	running, _ := ic.AppRunning(appName)
	return map[string]any{
		"name":    appName,
		"running": running,
	}, nil
}

func (ic *IntegrationContext) ElementVisible(selector string) (bool, error) {
	result, err := ic.toolExec(context.Background(), "desktop_find_element", map[string]any{
		"selector": selector,
	})
	if err != nil {
		return false, err
	}
	return result != "" && !contains(result, "not found"), nil
}

func (ic *IntegrationContext) Screenshot() ([]byte, error) {
	result, err := ic.toolExec(context.Background(), "desktop_screenshot", nil)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

func (ic *IntegrationContext) CustomVerify(name string, params map[string]any) (*VerificationResult, error) {
	return nil, fmt.Errorf("custom verify '%s' not implemented", name)
}

func (ie *IntegrationExecutor) ExecuteFromDesktopPlan(ctx context.Context, check *apps.DesktopPlanCheck) (*Result, error) {
	if check == nil {
		return &Result{Passed: true}, nil
	}
	timeout := 5 * time.Second
	if check.RetryDelayMS > 0 {
		timeout = time.Duration(check.RetryDelayMS) * time.Millisecond * time.Duration(check.Retry)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	vType := ie.mapToolToVerificationType(check.Tool)
	spec := &Spec{
		Condition: &Condition{
			Type:       vType,
			Parameters: mergeParams(check.Target, check.Input),
			Retry: &RetryConfig{
				MaxAttempts:   max(check.Retry, 1),
				InitialDelay:  500 * time.Millisecond,
				MaxDelay:      2 * time.Second,
				BackoffFactor: 1.5,
			},
		},
	}
	return ie.executor.Execute(ctx, spec)
}

func (ie *IntegrationExecutor) mapToolToVerificationType(toolName string) VerificationType {
	switch toolName {
	case "desktop_verify_text", "desktop_wait_text":
		return VerificationTypeTextContains
	case "desktop_ocr":
		return VerificationTypeOCRContains
	case "desktop_screenshot":
		return VerificationTypeScreenshot
	case "file_exists":
		return VerificationTypeFileExists
	case "clipboard_read":
		return VerificationTypeClipboard
	case "http_request":
		return VerificationTypeNetwork
	case "desktop_list_windows":
		return VerificationTypeWindowAppears
	case "desktop_find_element":
		return VerificationTypeElementVisible
	default:
		return VerificationTypeCustom
	}
}

func mergeParams(target, input map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range target {
		result[k] = v
	}
	for k, v := range input {
		result[k] = v
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
			(containsStr(s, substr)))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func match(pattern, s string) bool {
	if pattern == "" {
		return true
	}
	for i := 0; i < len(s); i++ {
		if i+len(pattern) <= len(s) && s[i:i+len(pattern)] == pattern {
			return true
		}
	}
	return false
}
