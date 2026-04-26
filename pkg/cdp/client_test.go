package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func stubRun(t *testing.T, fn func(context.Context, ...chromedp.Action) error) {
	t.Helper()
	original := runChromedp
	runChromedp = fn
	t.Cleanup(func() {
		runChromedp = original
	})
}

func TestAllocatorFlagOverridesHeadlessFalse(t *testing.T) {
	overrides := (&CDPOptions{Headless: false}).allocatorFlagOverrides()
	got := map[string]any{}
	for _, override := range overrides {
		got[override.name] = override.value
	}

	want := map[string]any{
		"headless":        false,
		"hide-scrollbars": false,
		"mute-audio":      false,
	}

	for key, value := range want {
		if got[key] != value {
			t.Fatalf("override %q = %v, want %v", key, got[key], value)
		}
	}
}

func TestAllocatorFlagOverridesHeadlessTrue(t *testing.T) {
	overrides := (&CDPOptions{
		Headless:      true,
		DisableImages: true,
		CacheDisabled: true,
	}).allocatorFlagOverrides()

	got := map[string]any{}
	for _, override := range overrides {
		got[override.name] = override.value
	}

	if _, exists := got["headless"]; exists {
		t.Fatal("unexpected headless override when Headless is true")
	}
	if got["disable-images"] != true {
		t.Fatalf("disable-images override = %v, want true", got["disable-images"])
	}
	if got["disk-cache-size"] != 0 {
		t.Fatalf("disk-cache-size override = %v, want 0", got["disk-cache-size"])
	}
}

func TestCombineCleanupRunsAllFuncs(t *testing.T) {
	var calls []string
	cleanup := combineCleanup(
		func() { calls = append(calls, "root") },
		nil,
		func() { calls = append(calls, "alloc") },
	)

	cleanup()

	if len(calls) != 2 || calls[0] != "root" || calls[1] != "alloc" {
		t.Fatalf("cleanup calls = %v, want [root alloc]", calls)
	}
}

func TestNewAllocatorContextReturnsCancelableContext(t *testing.T) {
	ctx, cleanup := newAllocatorContext(&CDPOptions{Headless: false})
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup")
	}

	cleanup()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected cleanup to cancel returned context")
	}

	if err := ctx.Err(); err == nil || err != context.Canceled {
		t.Fatalf("context error = %v, want %v", err, context.Canceled)
	}
}

func TestExtraHTTPHeadersSeparatesUserAgent(t *testing.T) {
	eb := &EnhancedBrowser{
		headers: map[string]string{
			"Authorization": "Bearer test-token",
			"X-Trace-ID":    "trace-123",
			"User-Agent":    "from-header",
		},
		userAgent: "custom-agent",
	}

	gotHeaders, gotUserAgent := eb.extraHTTPHeaders()
	wantHeaders := network.Headers{
		"Authorization": "Bearer test-token",
		"X-Trace-ID":    "trace-123",
	}

	if gotUserAgent != "custom-agent" {
		t.Fatalf("user agent = %q, want %q", gotUserAgent, "custom-agent")
	}
	if len(gotHeaders) != len(wantHeaders) {
		t.Fatalf("header count = %d, want %d", len(gotHeaders), len(wantHeaders))
	}
	for key, value := range wantHeaders {
		if gotHeaders[key] != value {
			t.Fatalf("header %q = %v, want %v", key, gotHeaders[key], value)
		}
	}
	if _, exists := gotHeaders["User-Agent"]; exists {
		t.Fatal("unexpected User-Agent header in extra HTTP headers")
	}
}

func TestJSStringLiteralRoundTrip(t *testing.T) {
	input := "key'with\"quotes\\and\nnewline"
	encoded := jsStringLiteral(input)

	var decoded string
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("encoded literal %q did not decode: %v", encoded, err)
	}
	if decoded != input {
		t.Fatalf("decoded literal = %q, want %q", decoded, input)
	}
}

func TestBrowserToolMethodsUseRunner(t *testing.T) {
	var calls int
	var lastCtx context.Context
	var lastActions int
	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		calls++
		lastCtx = ctx
		lastActions = len(actions)
		return nil
	})

	tool, err := NewBrowserTool(context.Background())
	if err != nil {
		t.Fatalf("NewBrowserTool error = %v", err)
	}

	if err := tool.Navigate("https://example.com"); err != nil {
		t.Fatalf("Navigate error = %v", err)
	}
	if calls == 0 || lastCtx == nil || lastActions != 2 {
		t.Fatalf("Navigate runner calls=%d ctx=%v actions=%d", calls, lastCtx, lastActions)
	}

	if _, err := tool.Screenshot(); err != nil {
		t.Fatalf("Screenshot error = %v", err)
	}
	if err := tool.Click("#submit"); err != nil {
		t.Fatalf("Click error = %v", err)
	}
	if err := tool.Type("#name", "anyclaw"); err != nil {
		t.Fatalf("Type error = %v", err)
	}
	if _, err := tool.GetElementText("#title"); err != nil {
		t.Fatalf("GetElementText error = %v", err)
	}
	if _, err := tool.GetElementAttribute("#title", "data-id"); err != nil {
		t.Fatalf("GetElementAttribute error = %v", err)
	}
	if _, err := tool.GetElementHTML("#title"); err != nil {
		t.Fatalf("GetElementHTML error = %v", err)
	}
	if err := tool.Scroll(10, 20); err != nil {
		t.Fatalf("Scroll error = %v", err)
	}
	if err := tool.ScrollToElement(`#weird'"slash\selector`); err != nil {
		t.Fatalf("ScrollToElement error = %v", err)
	}
	if _, err := tool.GetPageSource(); err != nil {
		t.Fatalf("GetPageSource error = %v", err)
	}
	if _, err := tool.GetURL(); err != nil {
		t.Fatalf("GetURL error = %v", err)
	}
	if _, err := tool.GetTitle(); err != nil {
		t.Fatalf("GetTitle error = %v", err)
	}
	if err := tool.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

func TestBrowserToolVisibilityAndWaitHelpers(t *testing.T) {
	tool := &BrowserTool{ctx: context.Background()}

	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		return nil
	})
	visible, err := tool.IsVisible("#ok")
	if err != nil || !visible {
		t.Fatalf("IsVisible success = (%v, %v), want (true, nil)", visible, err)
	}
	if err := tool.WaitForSelector("#ok", time.Second); err != nil {
		t.Fatalf("WaitForSelector error = %v", err)
	}
	if err := tool.WaitForNavigation(time.Second); err != nil {
		t.Fatalf("WaitForNavigation error = %v", err)
	}

	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		return errors.New("boom")
	})
	visible, err = tool.IsVisible("#missing")
	if err != nil || visible {
		t.Fatalf("IsVisible failure = (%v, %v), want (false, nil)", visible, err)
	}
}

func TestSelectorHelpers(t *testing.T) {
	if got := ResolveSelectorBy("//div", "xpath"); got != "//div" {
		t.Fatalf("ResolveSelectorBy xpath = %q", got)
	}
	if got := ResolveSelectorBy(".card", "css"); got != ".card" {
		t.Fatalf("ResolveSelectorBy css = %q", got)
	}
	if got := ResolveSelectorBy("#id", "unknown"); got != "#id" {
		t.Fatalf("ResolveSelectorBy default = %q", got)
	}

	tests := []struct {
		input string
		want  string
		kind  string
	}{
		{"//button", "//button", "xpath"},
		{"#submit", "#submit", "id"},
		{".primary", ".primary", "class"},
		{"name=value", `[name="value"]`, "css"},
		{"button", "button", "css"},
	}

	for _, tc := range tests {
		if got := ResolveCDPSelector(tc.input); got != tc.want {
			t.Fatalf("ResolveCDPSelector(%q) = %q, want %q", tc.input, got, tc.want)
		}
		parsedSelector, parsedKind := ParseSelector(tc.want)
		if parsedSelector != tc.want || parsedKind != tc.kind {
			t.Fatalf("ParseSelector(%q) = (%q, %q), want (%q, %q)", tc.want, parsedSelector, parsedKind, tc.want, tc.kind)
		}
	}
}

func TestCDPOptionsHelpers(t *testing.T) {
	opts := DefaultCDPOptions()
	if !opts.Headless || opts.WindowWidth != 1920 || opts.WindowHeight != 1080 {
		t.Fatalf("DefaultCDPOptions = %+v", opts)
	}

	allocOpts := (&CDPOptions{
		Headless:      false,
		WindowWidth:   1280,
		WindowHeight:  720,
		UserAgent:     "ua",
		Proxy:         "http://proxy",
		DisableImages: true,
		CacheDisabled: true,
	}).AllocatorOptions()

	if len(allocOpts) == 0 {
		t.Fatal("expected allocator options")
	}
}

func TestRunInContextInvokesCallback(t *testing.T) {
	called := false
	if err := RunInContext(context.Background(), &CDPOptions{Headless: false}, func(tool *BrowserTool) error {
		called = true
		if tool == nil {
			t.Fatal("expected tool")
		}
		return nil
	}); err != nil {
		t.Fatalf("RunInContext error = %v", err)
	}
	if !called {
		t.Fatal("expected callback to run")
	}
}

func TestEnhancedBrowserHelpersAndMethods(t *testing.T) {
	var calls int
	var actionCounts []int
	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		calls++
		actionCounts = append(actionCounts, len(actions))
		return nil
	})

	eb, err := NewEnhancedBrowser(&CDPOptions{Headless: false})
	if err != nil {
		t.Fatalf("NewEnhancedBrowser error = %v", err)
	}

	eb.SetHeader("Authorization", "Bearer abc")
	eb.SetUserAgent("custom-agent")
	if err := eb.Navigate("https://example.com"); err != nil {
		t.Fatalf("Navigate error = %v", err)
	}
	if err := eb.NavigateWithHeaders("https://example.com"); err != nil {
		t.Fatalf("NavigateWithHeaders error = %v", err)
	}
	if _, err := eb.GetLocalStorage(`k'"\`); err != nil {
		t.Fatalf("GetLocalStorage error = %v", err)
	}
	if err := eb.SetLocalStorage(`k'"\`, `v'"\`); err != nil {
		t.Fatalf("SetLocalStorage error = %v", err)
	}
	if err := eb.RemoveLocalStorage(`k'"\`); err != nil {
		t.Fatalf("RemoveLocalStorage error = %v", err)
	}
	if err := eb.ClearLocalStorage(); err != nil {
		t.Fatalf("ClearLocalStorage error = %v", err)
	}
	if _, err := eb.GetSessionStorage(`k'"\`); err != nil {
		t.Fatalf("GetSessionStorage error = %v", err)
	}
	if err := eb.SetSessionStorage(`k'"\`, `v'"\`); err != nil {
		t.Fatalf("SetSessionStorage error = %v", err)
	}
	if err := eb.ClearSessionStorage(); err != nil {
		t.Fatalf("ClearSessionStorage error = %v", err)
	}
	eb.ClearHeaders()
	if eb.headers != nil {
		t.Fatal("expected headers to be cleared")
	}
	if err := eb.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if calls < 9 {
		t.Fatalf("expected multiple runner calls, got %d", calls)
	}
	if len(actionCounts) < 2 || actionCounts[0] != 2 || actionCounts[1] < 4 {
		t.Fatalf("unexpected action counts: %v", actionCounts)
	}
}

func TestEnhancedBrowserGetAllLocalStorageErrorPaths(t *testing.T) {
	eb := &EnhancedBrowser{ctx: context.Background()}

	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		return errors.New("run failed")
	})
	if _, err := eb.GetAllLocalStorage(); err == nil {
		t.Fatal("expected runner error")
	}

	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		return nil
	})
	if _, err := eb.GetAllLocalStorage(); err == nil {
		t.Fatal("expected json decode error")
	}
}

func TestElementFinderAndFormHandlerMethods(t *testing.T) {
	var calls int
	stubRun(t, func(ctx context.Context, actions ...chromedp.Action) error {
		calls++
		return nil
	})

	finder := NewElementFinder(context.Background())
	if finder == nil {
		t.Fatal("expected finder")
	}
	if _, err := finder.FindByText(`quote'"\text`); err != nil {
		t.Fatalf("FindByText error = %v", err)
	}
	if got, err := finder.FindByAttribute("data-id", "42"); err != nil || got != `[data-id="42"]` {
		t.Fatalf("FindByAttribute = (%q, %v)", got, err)
	}
	if _, err := finder.FindByXPath(`//div[@data-x='"quoted"']`); err == nil {
		t.Fatal("expected FindByXPath to report not found without a real DOM result")
	}
	if _, err := finder.Count(`div[data-id="42"]`); err != nil {
		t.Fatalf("Count error = %v", err)
	}

	form := NewFormHandler(context.Background())
	if form == nil {
		t.Fatal("expected form handler")
	}
	if err := form.Fill("#profile", map[string]string{"name": "AnyClaw", "title": "Builder"}); err != nil {
		t.Fatalf("Fill error = %v", err)
	}
	if err := form.Submit("#profile"); err != nil {
		t.Fatalf("Submit error = %v", err)
	}
	if err := form.Reset("#profile"); err != nil {
		t.Fatalf("Reset error = %v", err)
	}
	if _, err := form.GetValues("#profile", []string{"name", "title"}); err != nil {
		t.Fatalf("GetValues error = %v", err)
	}
	if calls < 8 {
		t.Fatalf("expected runner calls, got %d", calls)
	}
}
