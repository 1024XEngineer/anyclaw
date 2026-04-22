package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	desktopexec "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
)

type noopVerificationContext struct{}

func (noopVerificationContext) FileExists(path string) (bool, error) { return false, nil }
func (noopVerificationContext) FileContains(path string, content string) (bool, error) {
	return false, nil
}
func (noopVerificationContext) FileMatches(path string, pattern string) (bool, error) {
	return false, nil
}
func (noopVerificationContext) WindowAppears(title string) (bool, error) { return false, nil }
func (noopVerificationContext) WindowFocused(title string) (bool, error) { return false, nil }
func (noopVerificationContext) TextContains(x, y int, width, height int, text string) (bool, error) {
	return false, nil
}
func (noopVerificationContext) OCRText(x, y int, width, height int) (string, error) { return "", nil }
func (noopVerificationContext) Clipboard() (string, error)                          { return "", nil }
func (noopVerificationContext) ClipboardContains(content string) (bool, bool)       { return false, false }
func (noopVerificationContext) NetworkRequest(url string) (int, error)              { return 0, nil }
func (noopVerificationContext) AppRunning(appName string) (bool, error)             { return false, nil }
func (noopVerificationContext) AppState(appName string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (noopVerificationContext) ElementVisible(selector string) (bool, error) { return false, nil }
func (noopVerificationContext) Screenshot() ([]byte, error)                  { return nil, nil }
func (noopVerificationContext) CustomVerify(name string, params map[string]any) (*VerificationResult, error) {
	return nil, errors.New("not implemented")
}

type networkContext struct {
	noopVerificationContext
	status int
}

func (n networkContext) NetworkRequest(url string) (int, error) {
	return n.status, nil
}

func TestQuotePowerShellLiteralEscapesSingleQuotes(t *testing.T) {
	got := quotePowerShellLiteral("foo'; Write-Host hacked; 'bar")
	want := "'foo''; Write-Host hacked; ''bar'"
	if got != want {
		t.Fatalf("quotePowerShellLiteral() = %q, want %q", got, want)
	}
}

func TestExecuteConditionStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec := NewVerificationExecutor(noopVerificationContext{})
	result := exec.executeCondition(ctx, &Condition{
		Type: VerificationTypeFileExists,
		Parameters: map[string]any{
			"path": "ignored",
		},
		Retry: &RetryConfig{
			MaxAttempts:  3,
			InitialDelay: time.Millisecond,
		},
	})

	if result.Passed {
		t.Fatal("expected canceled verification to fail")
	}
	if result.Error != context.Canceled.Error() {
		t.Fatalf("expected context canceled error, got %q", result.Error)
	}
}

func TestVerifyNetworkStatusAcceptsIntParameter(t *testing.T) {
	exec := NewVerificationExecutor(networkContext{status: 200})
	result := exec.verifyNetworkStatus(map[string]any{
		"url":    "https://example.com",
		"status": 200,
	})
	if !result.Passed {
		t.Fatalf("expected int status parameter to pass, got %#v", result)
	}
}

func TestExecuteFromDesktopPlanPropagatesContextAndSupportsScreenshot(t *testing.T) {
	type ctxKey string
	const key ctxKey = "trace"

	var (
		seenValue any
		seenTool  string
	)
	exec := NewIntegrationExecutor(func(ctx context.Context, toolName string, input map[string]any) (string, error) {
		seenValue = ctx.Value(key)
		seenTool = toolName
		if toolName != "desktop_screenshot" {
			t.Fatalf("unexpected tool invocation: %s", toolName)
		}
		return "image-bytes", nil
	})

	ctx := context.WithValue(context.Background(), key, "verification-trace")
	result, err := exec.ExecuteFromDesktopPlan(ctx, &desktopexec.DesktopPlanCheck{
		Tool:         "desktop_screenshot",
		Retry:        1,
		RetryDelayMS: 10,
	})
	if err != nil {
		t.Fatalf("ExecuteFromDesktopPlan failed: %v", err)
	}
	if !result.AllPassed() {
		t.Fatalf("expected screenshot verification to pass, got %#v", result)
	}
	if seenValue != "verification-trace" {
		t.Fatalf("expected context value to propagate, got %#v", seenValue)
	}
	if seenTool != "desktop_screenshot" {
		t.Fatalf("expected screenshot tool to be invoked, got %q", seenTool)
	}
}
