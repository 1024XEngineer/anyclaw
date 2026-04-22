package verification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scriptedVerificationContext struct {
	fileExistsResult       bool
	fileExistsErr          error
	fileContainsResult     bool
	fileContainsErr        error
	fileMatchesResult      bool
	fileMatchesErr         error
	windowAppearsResult    bool
	windowAppearsErr       error
	windowFocusedResult    bool
	windowFocusedErr       error
	textContainsResult     bool
	textContainsErr        error
	ocrTextResult          string
	ocrTextErr             error
	clipboardResult        string
	clipboardErr           error
	clipboardContainsHit   bool
	clipboardContainsValid bool
	networkStatus          int
	networkErr             error
	appRunningResult       bool
	appRunningErr          error
	appStateResult         map[string]any
	appStateErr            error
	elementVisibleResult   bool
	elementVisibleErr      error
	screenshotResult       []byte
	screenshotErr          error
	customResult           *VerificationResult
	customErr              error
}

func (c scriptedVerificationContext) FileExists(path string) (bool, error) {
	return c.fileExistsResult, c.fileExistsErr
}
func (c scriptedVerificationContext) FileContains(path string, content string) (bool, error) {
	return c.fileContainsResult, c.fileContainsErr
}
func (c scriptedVerificationContext) FileMatches(path string, pattern string) (bool, error) {
	return c.fileMatchesResult, c.fileMatchesErr
}
func (c scriptedVerificationContext) WindowAppears(title string) (bool, error) {
	return c.windowAppearsResult, c.windowAppearsErr
}
func (c scriptedVerificationContext) WindowFocused(title string) (bool, error) {
	return c.windowFocusedResult, c.windowFocusedErr
}
func (c scriptedVerificationContext) TextContains(x, y int, width, height int, text string) (bool, error) {
	return c.textContainsResult, c.textContainsErr
}
func (c scriptedVerificationContext) OCRText(x, y int, width, height int) (string, error) {
	return c.ocrTextResult, c.ocrTextErr
}
func (c scriptedVerificationContext) Clipboard() (string, error) {
	return c.clipboardResult, c.clipboardErr
}
func (c scriptedVerificationContext) ClipboardContains(content string) (bool, bool) {
	return c.clipboardContainsHit, c.clipboardContainsValid
}
func (c scriptedVerificationContext) NetworkRequest(url string) (int, error) {
	return c.networkStatus, c.networkErr
}
func (c scriptedVerificationContext) AppRunning(appName string) (bool, error) {
	return c.appRunningResult, c.appRunningErr
}
func (c scriptedVerificationContext) AppState(appName string) (map[string]any, error) {
	return c.appStateResult, c.appStateErr
}
func (c scriptedVerificationContext) ElementVisible(selector string) (bool, error) {
	return c.elementVisibleResult, c.elementVisibleErr
}
func (c scriptedVerificationContext) Screenshot() ([]byte, error) {
	return c.screenshotResult, c.screenshotErr
}
func (c scriptedVerificationContext) CustomVerify(name string, params map[string]any) (*VerificationResult, error) {
	return c.customResult, c.customErr
}

func TestVerificationHelpersAndRegistry(t *testing.T) {
	res := NewVerificationResult(VerificationTypeCustom, true).
		WithMessage("ok").
		WithEvidence("k", "v").
		WithActual("a", 1).
		WithExpected("b", 2).
		WithRetries(3)
	if !res.Passed || res.Message != "ok" || res.Evidence["k"] != "v" || res.Actual["a"] != 1 || res.Expected["b"] != 2 || res.Retries != 3 {
		t.Fatalf("unexpected verification result helper output: %#v", res)
	}

	if err := (&Spec{}).Validate(); err == nil {
		t.Fatal("expected empty spec validation to fail")
	}
	if err := (&Spec{Condition: &Condition{}, Composite: &CompositeCondition{}}).Validate(); err == nil {
		t.Fatal("expected mixed spec validation to fail")
	}

	result := &Result{
		Results: []*VerificationResult{
			NewVerificationResult(VerificationTypeFileExists, true),
			NewVerificationResult(VerificationTypeFileExists, false),
		},
	}
	if result.AllPassed() {
		t.Fatal("expected AllPassed to be false")
	}
	if !result.AnyPassed() {
		t.Fatal("expected AnyPassed to be true")
	}
	if got := result.GetSummary(); got != "passed: 1, failed: 1" {
		t.Fatalf("unexpected summary: %q", got)
	}

	registry := NewTemplateRegistry()
	registry.Register(&VerificationTemplate{
		Name:        "tmp",
		Description: "desc",
		Condition: &Condition{
			Type: VerificationTypeFileExists,
			Parameters: map[string]any{
				"path": "base",
			},
		},
	})
	list := registry.List()
	if len(list) != 1 || list[0].Name != "tmp" {
		t.Fatalf("unexpected template list: %#v", list)
	}
	tpl, err := registry.Get("tmp")
	if err != nil {
		t.Fatalf("Get template failed: %v", err)
	}
	applied := tpl.Apply(map[string]any{"path": "override"})
	if applied.Condition.Parameters["path"] != "override" {
		t.Fatalf("expected Apply override, got %#v", applied.Condition.Parameters)
	}
	if _, err := registry.Get("missing"); err == nil {
		t.Fatal("expected missing template lookup to fail")
	}
	if spec := (&VerificationTemplate{}).Apply(nil); spec == nil {
		t.Fatal("expected nil-condition Apply to return empty spec")
	}

	defaults := NewVerificationExecutorWithDefaults(noopVerificationContext{})
	if len(defaults.registry.List()) < 10 {
		t.Fatalf("expected default templates to be registered, got %d", len(defaults.registry.List()))
	}
}

func TestVerificationExecutorVerifyBranches(t *testing.T) {
	custom := NewVerificationResult(VerificationTypeCustom, true).WithMessage("custom-ok")
	exec := NewVerificationExecutor(scriptedVerificationContext{
		fileExistsResult:       true,
		fileContainsResult:     true,
		fileMatchesResult:      true,
		windowAppearsResult:    true,
		windowFocusedResult:    true,
		textContainsResult:     true,
		ocrTextResult:          "Hello World",
		clipboardResult:        "copied text",
		clipboardContainsHit:   true,
		clipboardContainsValid: true,
		networkStatus:          200,
		appRunningResult:       true,
		appStateResult: map[string]any{
			"phase": "ready",
		},
		elementVisibleResult: true,
		screenshotResult:     []byte("img"),
		customResult:         custom,
	})

	tests := []struct {
		name   string
		vType  VerificationType
		params map[string]any
	}{
		{"file-not-exists", VerificationTypeFileNotExists, map[string]any{"path": "a"}},
		{"file-matches", VerificationTypeFileMatches, map[string]any{"path": "a", "pattern": "x"}},
		{"window-not-exists", VerificationTypeWindowNotExists, map[string]any{"title": "win"}},
		{"window-focused", VerificationTypeWindowFocused, map[string]any{"title": "win"}},
		{"text-contains", VerificationTypeTextContains, map[string]any{"text": "hello", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0}},
		{"text-not-contains", VerificationTypeTextNotContains, map[string]any{"text": "missing", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0}},
		{"text-matches", VerificationTypeTextMatches, map[string]any{"pattern": "Hello", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0}},
		{"clipboard", VerificationTypeClipboard, nil},
		{"clipboard-contains", VerificationTypeClipboardContains, map[string]any{"content": "copied"}},
		{"network", VerificationTypeNetwork, map[string]any{"url": "https://example.com"}},
		{"network-status", VerificationTypeNetworkStatus, map[string]any{"url": "https://example.com", "status": "200"}},
		{"app-running", VerificationTypeAppRunning, map[string]any{"app": "calc"}},
		{"app-not-running", VerificationTypeAppNotRunning, map[string]any{"app": "calc"}},
		{"app-state", VerificationTypeAppState, map[string]any{"app": "calc", "key": "phase", "value": "ready"}},
		{"ocr-equals", VerificationTypeOCREquals, map[string]any{"expected": "Hello World", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0}},
		{"ocr-contains", VerificationTypeOCRContains, map[string]any{"expected": "hello", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0}},
		{"element-visible", VerificationTypeElementVisible, map[string]any{"selector": "#ok"}},
		{"element-not-visible", VerificationTypeElementNotVisible, map[string]any{"selector": "#missing"}},
		{"custom", VerificationTypeCustom, map[string]any{"name": "custom-check"}},
		{"screenshot", VerificationTypeScreenshot, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := exec.verify(context.Background(), tc.vType, tc.params)
			if result == nil {
				t.Fatal("expected non-nil verification result")
			}
		})
	}

	if got := exec.verify(context.Background(), VerificationType("unknown"), nil); got.Passed || got.Error == "" {
		t.Fatalf("expected unknown verification type failure, got %#v", got)
	}
	if got := truncate("abcdefghijklmnopqrstuvwxyz", 5); got != "abcde..." {
		t.Fatalf("unexpected truncate result: %q", got)
	}
}

func TestExecuteCompositeAndCancellationPaths(t *testing.T) {
	exec := NewVerificationExecutor(scriptedVerificationContext{
		fileExistsResult:   true,
		fileContainsResult: false,
	})

	andResults := exec.executeComposite(context.Background(), &CompositeCondition{
		Op: "and",
		Conditions: []Condition{
			{Type: VerificationTypeFileExists, Parameters: map[string]any{"path": "a"}},
			{Type: VerificationTypeFileContains, Parameters: map[string]any{"path": "a", "content": "x"}},
		},
	})
	if len(andResults) != 2 || andResults[0].Passed || andResults[1].Passed {
		t.Fatalf("expected AND composite to mark all failed, got %#v", andResults)
	}

	orResults := exec.executeComposite(context.Background(), &CompositeCondition{
		Op:  "or",
		Not: true,
		Conditions: []Condition{
			{Type: VerificationTypeFileExists, Parameters: map[string]any{"path": "a"}},
			{Type: VerificationTypeFileContains, Parameters: map[string]any{"path": "a", "content": "x"}},
		},
	})
	if len(orResults) != 2 || orResults[0].Passed || orResults[1].Passed {
		t.Fatalf("expected OR+NOT composite to invert results, got %#v", orResults)
	}

	ctx, cancel := context.WithCancel(context.Background())
	exec = NewVerificationExecutor(scriptedVerificationContext{fileExistsResult: false})
	done := make(chan *VerificationResult, 1)
	go func() {
		done <- exec.executeCondition(ctx, &Condition{
			Type:       VerificationTypeFileExists,
			Parameters: map[string]any{"path": "a"},
			Retry: &RetryConfig{
				MaxAttempts:  3,
				InitialDelay: 50 * time.Millisecond,
			},
		})
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	result := <-done
	if result.Error != context.Canceled.Error() {
		t.Fatalf("expected cancellation to surface, got %#v", result)
	}
}

func TestVerificationErrorBranches(t *testing.T) {
	exec := NewVerificationExecutor(scriptedVerificationContext{
		fileExistsErr:          errors.New("stat failed"),
		fileContainsErr:        errors.New("read failed"),
		fileMatchesErr:         errors.New("match failed"),
		windowAppearsErr:       errors.New("window failed"),
		windowFocusedErr:       errors.New("focus failed"),
		textContainsErr:        errors.New("text failed"),
		ocrTextErr:             errors.New("ocr failed"),
		clipboardErr:           errors.New("clipboard failed"),
		networkErr:             errors.New("network failed"),
		appRunningErr:          errors.New("app failed"),
		appStateErr:            errors.New("state failed"),
		elementVisibleErr:      errors.New("element failed"),
		screenshotErr:          errors.New("shot failed"),
		customErr:              errors.New("custom failed"),
		clipboardContainsValid: false,
	})

	cases := []struct {
		name   string
		result *VerificationResult
	}{
		{"file-exists-error", exec.verifyFileExists(map[string]any{"path": "a"})},
		{"file-contains-error", exec.verifyFileContains(map[string]any{"path": "a", "content": "x"})},
		{"file-matches-error", exec.verifyFileMatches(map[string]any{"path": "a", "pattern": "x"})},
		{"window-appears-error", exec.verifyWindowAppears(map[string]any{"title": "x"})},
		{"window-focused-error", exec.verifyWindowFocused(map[string]any{"title": "x"})},
		{"text-contains-error", exec.verifyTextContains(map[string]any{"text": "x", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0})},
		{"text-matches-error", exec.verifyTextMatches(map[string]any{"pattern": "x", "x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0})},
		{"clipboard-error", exec.verifyClipboard(nil)},
		{"clipboard-contains-error", exec.verifyClipboardContains(map[string]any{"content": "x"})},
		{"network-error", exec.verifyNetwork(map[string]any{"url": "https://example.com"})},
		{"app-running-error", exec.verifyAppRunning(map[string]any{"app": "calc"})},
		{"app-state-error", exec.verifyAppState(map[string]any{"app": "calc", "key": "phase", "value": "ready"})},
		{"element-visible-error", exec.verifyElementVisible(map[string]any{"selector": "#x"})},
		{"screenshot-error", exec.verifyScreenshot(nil)},
	}
	for _, tc := range cases {
		if tc.result == nil || tc.result.Error == "" {
			t.Fatalf("%s expected error result, got %#v", tc.name, tc.result)
		}
	}

	if got := exec.verifyCustom(map[string]any{"name": "check"}); got.Error == "" {
		t.Fatalf("expected custom verify error, got %#v", got)
	}
	if got := exec.verifyNetworkStatus(map[string]any{"url": "https://example.com"}); got.Error == "" {
		t.Fatalf("expected invalid status error, got %#v", got)
	}

	stateExec := NewVerificationExecutor(scriptedVerificationContext{
		appStateResult: map[string]any{"phase": "init"},
	})
	if got := stateExec.verifyAppState(map[string]any{"app": "calc", "key": "missing", "value": "ready"}); got.Error == "" {
		t.Fatalf("expected missing state key error, got %#v", got)
	}
}
