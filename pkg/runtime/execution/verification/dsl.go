package verification

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type VerificationType string

const (
	VerificationTypeFileExists        VerificationType = "file-exists"
	VerificationTypeFileNotExists     VerificationType = "file-not-exists"
	VerificationTypeFileContains      VerificationType = "file-contains"
	VerificationTypeFileMatches       VerificationType = "file-matches"
	VerificationTypeWindowAppears     VerificationType = "window-appears"
	VerificationTypeWindowNotExists   VerificationType = "window-not-exists"
	VerificationTypeWindowFocused     VerificationType = "window-focused"
	VerificationTypeTextContains      VerificationType = "text-contains"
	VerificationTypeTextNotContains   VerificationType = "text-not-contains"
	VerificationTypeTextMatches       VerificationType = "text-matches"
	VerificationTypeClipboard         VerificationType = "clipboard"
	VerificationTypeClipboardContains VerificationType = "clipboard-contains"
	VerificationTypeNetwork           VerificationType = "network"
	VerificationTypeNetworkStatus     VerificationType = "network-status"
	VerificationTypeAppRunning        VerificationType = "app-running"
	VerificationTypeAppNotRunning     VerificationType = "app-not-running"
	VerificationTypeAppState          VerificationType = "app-state"
	VerificationTypeOCREquals         VerificationType = "ocr-equals"
	VerificationTypeOCRContains       VerificationType = "ocr-contains"
	VerificationTypeElementVisible    VerificationType = "element-visible"
	VerificationTypeElementNotVisible VerificationType = "element-not-visible"
	VerificationTypeElementEnabled    VerificationType = "element-enabled"
	VerificationTypeScreenshot        VerificationType = "screenshot"
	VerificationTypeCustom            VerificationType = "custom"
	VerificationTypeAnd               VerificationType = "and"
	VerificationTypeOr                VerificationType = "or"
	VerificationTypeNot               VerificationType = "not"
)

type Condition struct {
	Type       VerificationType `json:"type"`
	Parameters map[string]any   `json:"parameters,omitempty"`
	Timeout    time.Duration    `json:"timeout,omitempty"`
	Retry      *RetryConfig     `json:"retry,omitempty"`
}

type RetryConfig struct {
	MaxAttempts   int           `json:"max_attempts"`
	InitialDelay  time.Duration `json:"initial_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	BackoffFactor float64       `json:"backoff_factor"`
}

type CompositeCondition struct {
	Op         string      `json:"op"`
	Conditions []Condition `json:"conditions"`
	Not        bool        `json:"not,omitempty"`
}

type VerificationResult struct {
	Passed    bool             `json:"passed"`
	Type      VerificationType `json:"type"`
	Message   string           `json:"message,omitempty"`
	Evidence  map[string]any   `json:"evidence,omitempty"`
	Actual    map[string]any   `json:"actual,omitempty"`
	Expected  map[string]any   `json:"expected,omitempty"`
	Duration  time.Duration    `json:"duration"`
	Timestamp time.Time        `json:"timestamp"`
	Retries   int              `json:"retries,omitempty"`
	Error     string           `json:"error,omitempty"`
}

func NewVerificationResult(verificationType VerificationType, passed bool) *VerificationResult {
	return &VerificationResult{
		Type:      verificationType,
		Passed:    passed,
		Evidence:  make(map[string]any),
		Actual:    make(map[string]any),
		Expected:  make(map[string]any),
		Timestamp: time.Now().UTC(),
	}
}

func (v *VerificationResult) WithMessage(msg string) *VerificationResult {
	v.Message = msg
	return v
}

func (v *VerificationResult) WithEvidence(key string, value any) *VerificationResult {
	v.Evidence[key] = value
	return v
}

func (v *VerificationResult) WithActual(key string, value any) *VerificationResult {
	v.Actual[key] = value
	return v
}

func (v *VerificationResult) WithExpected(key string, value any) *VerificationResult {
	v.Expected[key] = value
	return v
}

func (v *VerificationResult) WithError(err string) *VerificationResult {
	v.Error = err
	v.Passed = false
	return v
}

func (v *VerificationResult) WithRetries(retries int) *VerificationResult {
	v.Retries = retries
	return v
}

type Spec struct {
	Condition  *Condition          `json:"condition,omitempty"`
	Composite  *CompositeCondition `json:"composite,omitempty"`
	Template   string              `json:"template,omitempty"`
	Parameters map[string]any      `json:"parameters,omitempty"`
	Timeout    time.Duration       `json:"timeout,omitempty"`
	OnFailure  string              `json:"on_failure,omitempty"`
}

func (s *Spec) Validate() error {
	if s.Composite == nil && s.Condition == nil && s.Template == "" {
		return fmt.Errorf("verification spec must have condition, composite, or template")
	}
	if s.Composite != nil && s.Condition != nil {
		return fmt.Errorf("verification spec cannot have both condition and composite")
	}
	return nil
}

type Result struct {
	Passed    bool                  `json:"passed"`
	Results   []*VerificationResult `json:"results,omitempty"`
	Message   string                `json:"message,omitempty"`
	Summary   string                `json:"summary,omitempty"`
	Duration  time.Duration         `json:"duration"`
	Timestamp time.Time             `json:"timestamp"`
}

func (r *Result) AllPassed() bool {
	for _, res := range r.Results {
		if !res.Passed {
			return false
		}
	}
	return len(r.Results) > 0
}

func (r *Result) AnyPassed() bool {
	for _, res := range r.Results {
		if res.Passed {
			return true
		}
	}
	return false
}

func (r *Result) GetSummary() string {
	if len(r.Results) == 0 {
		return "no verifications performed"
	}
	passed := 0
	failed := 0
	for _, res := range r.Results {
		if res.Passed {
			passed++
		} else {
			failed++
		}
	}
	return fmt.Sprintf("passed: %d, failed: %d", passed, failed)
}

type Context interface {
	FileExists(path string) (bool, error)
	FileContains(path string, content string) (bool, error)
	FileMatches(path string, pattern string) (bool, error)
	WindowAppears(title string) (bool, error)
	WindowFocused(title string) (bool, error)
	TextContains(x, y int, width, height int, text string) (bool, error)
	OCRText(x, y int, width, height int) (string, error)
	Clipboard() (string, error)
	ClipboardContains(content string) (bool, bool)
	NetworkRequest(url string) (int, error)
	AppRunning(appName string) (bool, error)
	AppState(appName string) (map[string]any, error)
	ElementVisible(selector string) (bool, error)
	Screenshot() ([]byte, error)
	CustomVerify(name string, params map[string]any) (*VerificationResult, error)
}

type TemplateRegistry struct {
	templates map[string]*VerificationTemplate
}

type VerificationTemplate struct {
	Name        string
	Description string
	Condition   *Condition
}

func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string]*VerificationTemplate),
	}
}

func (r *TemplateRegistry) Register(template *VerificationTemplate) {
	r.templates[template.Name] = template
}

func (r *TemplateRegistry) Get(name string) (*VerificationTemplate, error) {
	template, ok := r.templates[name]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", name)
	}
	return template, nil
}

func (r *TemplateRegistry) List() []*VerificationTemplate {
	templates := make([]*VerificationTemplate, 0, len(r.templates))
	for _, t := range r.templates {
		templates = append(templates, t)
	}
	return templates
}

func (t *VerificationTemplate) Apply(params map[string]any) *Spec {
	if t.Condition == nil {
		return &Spec{}
	}
	applied := &Spec{
		Condition: &Condition{
			Type:       t.Condition.Type,
			Parameters: make(map[string]any),
			Timeout:    t.Condition.Timeout,
			Retry:      t.Condition.Retry,
		},
	}
	for k, v := range t.Condition.Parameters {
		applied.Condition.Parameters[k] = v
	}
	for k, v := range params {
		applied.Condition.Parameters[k] = v
	}
	return applied
}

type VerificationExecutor struct {
	context  Context
	registry *TemplateRegistry
}

func NewVerificationExecutor(ctx Context) *VerificationExecutor {
	return &VerificationExecutor{
		context:  ctx,
		registry: NewTemplateRegistry(),
	}
}

func (e *VerificationExecutor) Execute(ctx context.Context, spec *Spec) (*Result, error) {
	start := time.Now()
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	var results []*VerificationResult
	if spec.Template != "" {
		template, err := e.registry.Get(spec.Template)
		if err != nil {
			return nil, err
		}
		spec = template.Apply(spec.Parameters)
	}
	if spec.Composite != nil {
		compResults := e.executeComposite(ctx, spec.Composite)
		results = append(results, compResults...)
	} else if spec.Condition != nil {
		result := e.executeCondition(ctx, spec.Condition)
		results = append(results, result)
	}
	return &Result{
		Results:   results,
		Duration:  time.Since(start),
		Timestamp: time.Now().UTC(),
	}, nil
}

func (e *VerificationExecutor) executeComposite(ctx context.Context, composite *CompositeCondition) []*VerificationResult {
	var results []*VerificationResult
	for _, cond := range composite.Conditions {
		result := e.executeCondition(ctx, &cond)
		results = append(results, result)
	}
	if composite.Op == "and" {
		passed := true
		for _, r := range results {
			if !r.Passed {
				passed = false
				break
			}
		}
		for _, r := range results {
			r.Passed = passed
		}
	} else if composite.Op == "or" {
		passed := false
		for _, r := range results {
			if r.Passed {
				passed = true
				break
			}
		}
		for _, r := range results {
			r.Passed = passed
		}
	}
	if composite.Not {
		for _, r := range results {
			r.Passed = !r.Passed
		}
	}
	return results
}

func (e *VerificationExecutor) executeCondition(ctx context.Context, condition *Condition) *VerificationResult {
	start := time.Now()
	maxRetries := 1
	retryDelay := time.Second
	if condition.Retry != nil {
		maxRetries = condition.Retry.MaxAttempts
		retryDelay = condition.Retry.InitialDelay
	}
	var lastResult *VerificationResult
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return NewVerificationResult(condition.Type, false).
				WithError(err.Error()).
				WithRetries(attempt).
				WithMessage("verification canceled")
		}
		result := e.verify(ctx, condition.Type, condition.Parameters)
		if result.Passed {
			result.Retries = attempt
			result.Duration = time.Since(start)
			return result
		}
		lastResult = result
		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				lastResult = NewVerificationResult(condition.Type, false).
					WithError(ctx.Err().Error()).
					WithRetries(attempt + 1).
					WithMessage("verification canceled")
				lastResult.Duration = time.Since(start)
				return lastResult
			case <-time.After(retryDelay):
				retryDelay = time.Duration(float64(retryDelay) * 1.5)
				if retryDelay > 30*time.Second {
					retryDelay = 30 * time.Second
				}
			}
		}
	}
	lastResult.Retries = maxRetries
	lastResult.Duration = time.Since(start)
	return lastResult
}

func (e *VerificationExecutor) verify(ctx context.Context, vType VerificationType, params map[string]any) *VerificationResult {
	switch vType {
	case VerificationTypeFileExists:
		return e.verifyFileExists(params)
	case VerificationTypeFileNotExists:
		return e.verifyFileNotExists(params)
	case VerificationTypeFileContains:
		return e.verifyFileContains(params)
	case VerificationTypeFileMatches:
		return e.verifyFileMatches(params)
	case VerificationTypeWindowAppears:
		return e.verifyWindowAppears(params)
	case VerificationTypeWindowNotExists:
		return e.verifyWindowNotExists(params)
	case VerificationTypeWindowFocused:
		return e.verifyWindowFocused(params)
	case VerificationTypeTextContains:
		return e.verifyTextContains(params)
	case VerificationTypeTextNotContains:
		return e.verifyTextNotContains(params)
	case VerificationTypeTextMatches:
		return e.verifyTextMatches(params)
	case VerificationTypeClipboard:
		return e.verifyClipboard(params)
	case VerificationTypeClipboardContains:
		return e.verifyClipboardContains(params)
	case VerificationTypeNetwork:
		return e.verifyNetwork(params)
	case VerificationTypeNetworkStatus:
		return e.verifyNetworkStatus(params)
	case VerificationTypeAppRunning:
		return e.verifyAppRunning(params)
	case VerificationTypeAppNotRunning:
		return e.verifyAppNotRunning(params)
	case VerificationTypeAppState:
		return e.verifyAppState(params)
	case VerificationTypeOCREquals:
		return e.verifyOCREquals(params)
	case VerificationTypeOCRContains:
		return e.verifyOCRContains(params)
	case VerificationTypeElementVisible:
		return e.verifyElementVisible(params)
	case VerificationTypeElementNotVisible:
		return e.verifyElementNotVisible(params)
	case VerificationTypeScreenshot:
		return e.verifyScreenshot(params)
	case VerificationTypeCustom:
		return e.verifyCustom(params)
	default:
		return NewVerificationResult(vType, false).WithError(fmt.Sprintf("unknown verification type: %s", vType))
	}
}

func (e *VerificationExecutor) verifyFileExists(params map[string]any) *VerificationResult {
	path, _ := params["path"].(string)
	if path == "" {
		return NewVerificationResult(VerificationTypeFileExists, false).WithError("missing path parameter")
	}
	exists, err := e.context.FileExists(path)
	if err != nil {
		return NewVerificationResult(VerificationTypeFileExists, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeFileExists, exists)
	result.WithActual("exists", exists)
	result.WithExpected("path", path)
	if !exists {
		result.Message = fmt.Sprintf("file does not exist: %s", path)
	} else {
		result.Message = fmt.Sprintf("file exists: %s", path)
	}
	return result
}

func (e *VerificationExecutor) verifyFileNotExists(params map[string]any) *VerificationResult {
	path, _ := params["path"].(string)
	if path == "" {
		return NewVerificationResult(VerificationTypeFileNotExists, false).WithError("missing path parameter")
	}
	exists, err := e.context.FileExists(path)
	if err != nil {
		return NewVerificationResult(VerificationTypeFileNotExists, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeFileNotExists, !exists)
	result.WithActual("exists", exists)
	result.WithExpected("path", path)
	if exists {
		result.Message = fmt.Sprintf("file should not exist but does: %s", path)
	} else {
		result.Message = fmt.Sprintf("file does not exist as expected: %s", path)
	}
	return result
}

func (e *VerificationExecutor) verifyFileContains(params map[string]any) *VerificationResult {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" {
		return NewVerificationResult(VerificationTypeFileContains, false).WithError("missing path parameter")
	}
	if content == "" {
		return NewVerificationResult(VerificationTypeFileContains, false).WithError("missing content parameter")
	}
	contains, err := e.context.FileContains(path, content)
	if err != nil {
		return NewVerificationResult(VerificationTypeFileContains, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeFileContains, contains)
	result.WithActual("contains", contains)
	result.WithExpected("content", content)
	result.WithMessage(fmt.Sprintf("file %s contains expected content: %v", path, contains))
	return result
}

func (e *VerificationExecutor) verifyFileMatches(params map[string]any) *VerificationResult {
	path, _ := params["path"].(string)
	pattern, _ := params["pattern"].(string)
	if path == "" {
		return NewVerificationResult(VerificationTypeFileMatches, false).WithError("missing path parameter")
	}
	if pattern == "" {
		return NewVerificationResult(VerificationTypeFileMatches, false).WithError("missing pattern parameter")
	}
	matched, err := e.context.FileMatches(path, pattern)
	if err != nil {
		return NewVerificationResult(VerificationTypeFileMatches, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeFileMatches, matched)
	result.WithActual("matches", matched)
	result.WithExpected("pattern", pattern)
	result.WithMessage(fmt.Sprintf("file %s matches pattern %s: %v", path, pattern, matched))
	return result
}

func (e *VerificationExecutor) verifyWindowAppears(params map[string]any) *VerificationResult {
	title, _ := params["title"].(string)
	if title == "" {
		return NewVerificationResult(VerificationTypeWindowAppears, false).WithError("missing title parameter")
	}
	appears, err := e.context.WindowAppears(title)
	if err != nil {
		return NewVerificationResult(VerificationTypeWindowAppears, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeWindowAppears, appears)
	result.WithActual("appears", appears)
	result.WithExpected("title", title)
	if !appears {
		result.Message = fmt.Sprintf("window with title '%s' did not appear", title)
	} else {
		result.Message = fmt.Sprintf("window with title '%s' appeared", title)
	}
	return result
}

func (e *VerificationExecutor) verifyWindowNotExists(params map[string]any) *VerificationResult {
	title, _ := params["title"].(string)
	if title == "" {
		return NewVerificationResult(VerificationTypeWindowNotExists, false).WithError("missing title parameter")
	}
	appears, err := e.context.WindowAppears(title)
	if err != nil {
		return NewVerificationResult(VerificationTypeWindowNotExists, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeWindowNotExists, !appears)
	result.WithActual("exists", appears)
	result.WithExpected("title", title)
	if appears {
		result.Message = fmt.Sprintf("window '%s' should not exist but does", title)
	} else {
		result.Message = fmt.Sprintf("window '%s' does not exist as expected", title)
	}
	return result
}

func (e *VerificationExecutor) verifyWindowFocused(params map[string]any) *VerificationResult {
	title, _ := params["title"].(string)
	if title == "" {
		return NewVerificationResult(VerificationTypeWindowFocused, false).WithError("missing title parameter")
	}
	focused, err := e.context.WindowFocused(title)
	if err != nil {
		return NewVerificationResult(VerificationTypeWindowFocused, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeWindowFocused, focused)
	result.WithActual("focused", focused)
	result.WithExpected("title", title)
	if !focused {
		result.Message = fmt.Sprintf("window '%s' is not focused", title)
	} else {
		result.Message = fmt.Sprintf("window '%s' is focused", title)
	}
	return result
}

func (e *VerificationExecutor) verifyTextContains(params map[string]any) *VerificationResult {
	text, _ := params["text"].(string)
	x, _ := params["x"].(float64)
	y, _ := params["y"].(float64)
	width, _ := params["width"].(float64)
	height, _ := params["height"].(float64)
	if text == "" {
		return NewVerificationResult(VerificationTypeTextContains, false).WithError("missing text parameter")
	}
	contains, err := e.context.TextContains(int(x), int(y), int(width), int(height), text)
	if err != nil {
		return NewVerificationResult(VerificationTypeTextContains, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeTextContains, contains)
	result.WithActual("contains", contains)
	result.WithExpected("text", text)
	result.WithMessage(fmt.Sprintf("text '%s' found in region: %v", text, contains))
	return result
}

func (e *VerificationExecutor) verifyTextNotContains(params map[string]any) *VerificationResult {
	text, _ := params["text"].(string)
	x, _ := params["x"].(float64)
	y, _ := params["y"].(float64)
	width, _ := params["width"].(float64)
	height, _ := params["height"].(float64)
	if text == "" {
		return NewVerificationResult(VerificationTypeTextNotContains, false).WithError("missing text parameter")
	}
	contains, err := e.context.TextContains(int(x), int(y), int(width), int(height), text)
	if err != nil {
		return NewVerificationResult(VerificationTypeTextNotContains, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeTextNotContains, !contains)
	result.WithActual("contains", contains)
	result.WithExpected("text", text)
	if contains {
		result.Message = fmt.Sprintf("text '%s' should not be present but was found", text)
	} else {
		result.Message = fmt.Sprintf("text '%s' is not present as expected", text)
	}
	return result
}

func (e *VerificationExecutor) verifyTextMatches(params map[string]any) *VerificationResult {
	pattern, _ := params["pattern"].(string)
	x, _ := params["x"].(float64)
	y, _ := params["y"].(float64)
	width, _ := params["width"].(float64)
	height, _ := params["height"].(float64)
	if pattern == "" {
		return NewVerificationResult(VerificationTypeTextMatches, false).WithError("missing pattern parameter")
	}
	actualText, err := e.context.OCRText(int(x), int(y), int(width), int(height))
	if err != nil {
		return NewVerificationResult(VerificationTypeTextMatches, false).WithError(err.Error())
	}
	matched, _ := regexp.MatchString(pattern, actualText)
	result := NewVerificationResult(VerificationTypeTextMatches, matched)
	result.WithActual("text", actualText)
	result.WithExpected("pattern", pattern)
	result.WithMessage(fmt.Sprintf("text matches pattern '%s': %v (actual: '%s')", pattern, matched, actualText))
	return result
}

func (e *VerificationExecutor) verifyClipboard(params map[string]any) *VerificationResult {
	content, err := e.context.Clipboard()
	if err != nil {
		return NewVerificationResult(VerificationTypeClipboard, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeClipboard, content != "")
	result.WithActual("content", content)
	result.WithMessage(fmt.Sprintf("clipboard content: %s", truncate(content, 100)))
	return result
}

func (e *VerificationExecutor) verifyClipboardContains(params map[string]any) *VerificationResult {
	content, _ := params["content"].(string)
	if content == "" {
		return NewVerificationResult(VerificationTypeClipboardContains, false).WithError("missing content parameter")
	}
	found, valid := e.context.ClipboardContains(content)
	if !valid {
		return NewVerificationResult(VerificationTypeClipboardContains, false).WithError("clipboard is empty or error")
	}
	result := NewVerificationResult(VerificationTypeClipboardContains, found)
	result.WithActual("contains", found)
	result.WithExpected("content", content)
	if !found {
		result.Message = fmt.Sprintf("clipboard does not contain '%s'", truncate(content, 50))
	} else {
		result.Message = fmt.Sprintf("clipboard contains expected content")
	}
	return result
}

func (e *VerificationExecutor) verifyNetwork(params map[string]any) *VerificationResult {
	url, _ := params["url"].(string)
	if url == "" {
		return NewVerificationResult(VerificationTypeNetwork, false).WithError("missing url parameter")
	}
	statusCode, err := e.context.NetworkRequest(url)
	if err != nil {
		return NewVerificationResult(VerificationTypeNetwork, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeNetwork, statusCode >= 200 && statusCode < 400)
	result.WithActual("status_code", statusCode)
	result.WithExpected("status_range", "200-399")
	if statusCode >= 200 && statusCode < 400 {
		result.Message = fmt.Sprintf("network request to %s succeeded with status %d", url, statusCode)
	} else {
		result.Message = fmt.Sprintf("network request to %s failed with status %d", url, statusCode)
	}
	return result
}

func (e *VerificationExecutor) verifyNetworkStatus(params map[string]any) *VerificationResult {
	url, _ := params["url"].(string)
	if url == "" {
		return NewVerificationResult(VerificationTypeNetworkStatus, false).WithError("missing url parameter")
	}
	expectedStatus, ok := intParam(params["status"])
	if !ok {
		return NewVerificationResult(VerificationTypeNetworkStatus, false).WithError("missing or invalid status parameter")
	}
	statusCode, err := e.context.NetworkRequest(url)
	if err != nil {
		return NewVerificationResult(VerificationTypeNetworkStatus, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeNetworkStatus, expectedStatus == statusCode)
	result.WithActual("status_code", statusCode)
	result.WithExpected("status", expectedStatus)
	result.WithMessage(fmt.Sprintf("network status %d matches expected %d", statusCode, expectedStatus))
	return result
}

func (e *VerificationExecutor) verifyAppRunning(params map[string]any) *VerificationResult {
	appName, _ := params["app"].(string)
	if appName == "" {
		return NewVerificationResult(VerificationTypeAppRunning, false).WithError("missing app parameter")
	}
	running, err := e.context.AppRunning(appName)
	if err != nil {
		return NewVerificationResult(VerificationTypeAppRunning, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeAppRunning, running)
	result.WithActual("running", running)
	result.WithExpected("app", appName)
	if running {
		result.Message = fmt.Sprintf("app '%s' is running", appName)
	} else {
		result.Message = fmt.Sprintf("app '%s' is not running", appName)
	}
	return result
}

func (e *VerificationExecutor) verifyAppNotRunning(params map[string]any) *VerificationResult {
	appName, _ := params["app"].(string)
	if appName == "" {
		return NewVerificationResult(VerificationTypeAppNotRunning, false).WithError("missing app parameter")
	}
	running, err := e.context.AppRunning(appName)
	if err != nil {
		return NewVerificationResult(VerificationTypeAppNotRunning, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeAppNotRunning, !running)
	result.WithActual("running", running)
	result.WithExpected("app", appName)
	if running {
		result.Message = fmt.Sprintf("app '%s' should not be running but is", appName)
	} else {
		result.Message = fmt.Sprintf("app '%s' is not running as expected", appName)
	}
	return result
}

func (e *VerificationExecutor) verifyAppState(params map[string]any) *VerificationResult {
	appName, _ := params["app"].(string)
	key, _ := params["key"].(string)
	expectedValue, _ := params["value"].(any)
	if appName == "" {
		return NewVerificationResult(VerificationTypeAppState, false).WithError("missing app parameter")
	}
	state, err := e.context.AppState(appName)
	if err != nil {
		return NewVerificationResult(VerificationTypeAppState, false).WithError(err.Error())
	}
	actualValue, exists := state[key]
	if !exists {
		return NewVerificationResult(VerificationTypeAppState, false).WithError(fmt.Sprintf("key '%s' not found in app state", key))
	}
	matched := fmt.Sprintf("%v", actualValue) == fmt.Sprintf("%v", expectedValue)
	result := NewVerificationResult(VerificationTypeAppState, matched)
	result.WithActual(key, actualValue)
	result.WithExpected(key, expectedValue)
	result.WithMessage(fmt.Sprintf("app state '%s' = '%v' (expected: '%v')", key, actualValue, expectedValue))
	return result
}

func (e *VerificationExecutor) verifyOCREquals(params map[string]any) *VerificationResult {
	expected, _ := params["expected"].(string)
	x, _ := params["x"].(float64)
	y, _ := params["y"].(float64)
	width, _ := params["width"].(float64)
	height, _ := params["height"].(float64)
	if expected == "" {
		return NewVerificationResult(VerificationTypeOCREquals, false).WithError("missing expected parameter")
	}
	actual, err := e.context.OCRText(int(x), int(y), int(width), int(height))
	if err != nil {
		return NewVerificationResult(VerificationTypeOCREquals, false).WithError(err.Error())
	}
	matched := strings.TrimSpace(actual) == strings.TrimSpace(expected)
	result := NewVerificationResult(VerificationTypeOCREquals, matched)
	result.WithActual("text", actual)
	result.WithExpected("text", expected)
	result.WithMessage(fmt.Sprintf("OCR text equals expected: %v", matched))
	return result
}

func (e *VerificationExecutor) verifyOCRContains(params map[string]any) *VerificationResult {
	expected, _ := params["expected"].(string)
	x, _ := params["x"].(float64)
	y, _ := params["y"].(float64)
	width, _ := params["width"].(float64)
	height, _ := params["height"].(float64)
	if expected == "" {
		return NewVerificationResult(VerificationTypeOCRContains, false).WithError("missing expected parameter")
	}
	actual, err := e.context.OCRText(int(x), int(y), int(width), int(height))
	if err != nil {
		return NewVerificationResult(VerificationTypeOCRContains, false).WithError(err.Error())
	}
	contains := strings.Contains(strings.ToLower(actual), strings.ToLower(expected))
	result := NewVerificationResult(VerificationTypeOCRContains, contains)
	result.WithActual("text", actual)
	result.WithExpected("contains", expected)
	result.WithMessage(fmt.Sprintf("OCR text contains expected: %v", contains))
	return result
}

func (e *VerificationExecutor) verifyElementVisible(params map[string]any) *VerificationResult {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return NewVerificationResult(VerificationTypeElementVisible, false).WithError("missing selector parameter")
	}
	visible, err := e.context.ElementVisible(selector)
	if err != nil {
		return NewVerificationResult(VerificationTypeElementVisible, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeElementVisible, visible)
	result.WithActual("visible", visible)
	result.WithExpected("selector", selector)
	result.WithMessage(fmt.Sprintf("element '%s' visible: %v", selector, visible))
	return result
}

func (e *VerificationExecutor) verifyElementNotVisible(params map[string]any) *VerificationResult {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return NewVerificationResult(VerificationTypeElementNotVisible, false).WithError("missing selector parameter")
	}
	visible, err := e.context.ElementVisible(selector)
	if err != nil {
		return NewVerificationResult(VerificationTypeElementNotVisible, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeElementNotVisible, !visible)
	result.WithActual("visible", visible)
	result.WithExpected("selector", selector)
	result.WithMessage(fmt.Sprintf("element '%s' not visible as expected: %v", selector, !visible))
	return result
}

func (e *VerificationExecutor) verifyScreenshot(params map[string]any) *VerificationResult {
	data, err := e.context.Screenshot()
	if err != nil {
		return NewVerificationResult(VerificationTypeScreenshot, false).WithError(err.Error())
	}
	result := NewVerificationResult(VerificationTypeScreenshot, len(data) > 0)
	result.WithActual("bytes", len(data))
	result.WithMessage(fmt.Sprintf("captured screenshot with %d bytes", len(data)))
	if len(data) == 0 {
		result.Passed = false
		result.Error = "empty screenshot"
	}
	return result
}

func (e *VerificationExecutor) verifyCustom(params map[string]any) *VerificationResult {
	name, _ := params["name"].(string)
	if name == "" {
		return NewVerificationResult(VerificationTypeCustom, false).WithError("missing name parameter")
	}
	result, err := e.context.CustomVerify(name, params)
	if err != nil {
		return NewVerificationResult(VerificationTypeCustom, false).WithError(err.Error())
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func intParam(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
}
