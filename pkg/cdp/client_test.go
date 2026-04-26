package cdp

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/network"
)

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
