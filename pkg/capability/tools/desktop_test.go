package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopOpenCommandVariants(t *testing.T) {
	if got := desktopOpenCommand("https://example.com", "url"); !strings.Contains(got, "opened url") {
		t.Fatalf("expected URL open command, got %q", got)
	}
	if got := desktopOpenCommand("notes.txt", "file"); !strings.Contains(got, "opened file") {
		t.Fatalf("expected file open command, got %q", got)
	}
	if got := desktopOpenCommand("notepad", "app"); !strings.Contains(got, "started app") {
		t.Fatalf("expected app start command, got %q", got)
	}
	if got := desktopOpenCommand("anything", ""); !strings.Contains(got, "opened target") {
		t.Fatalf("expected default open command, got %q", got)
	}
}

func TestDesktopHelpersEscapeAndFormatKeys(t *testing.T) {
	if got := powerShellString("it's fine"); got != "'it''s fine'" {
		t.Fatalf("unexpected PowerShell string encoding: %q", got)
	}
	if got := sendKeysEscape("{a}+^%~()[]"); !strings.Contains(got, "{{}") || !strings.Contains(got, "{+}") {
		t.Fatalf("expected SendKeys escaping, got %q", got)
	}
	if got := formatSendKey("enter"); got != "{ENTER}" {
		t.Fatalf("expected ENTER key format, got %q", got)
	}
	if got := hotkeyToSendKeys([]string{"ctrl", "shift", "A"}); got != "^+A" {
		t.Fatalf("unexpected hotkey encoding: %q", got)
	}
	if got := hotkeyToSendKeys([]string{"win", "tab"}); !strings.Contains(got, "{LWIN}") || !strings.Contains(got, "{TAB}") {
		t.Fatalf("unexpected Windows hotkey encoding: %q", got)
	}
}

func TestDesktopMouseHelpers(t *testing.T) {
	down, up, err := mouseFlags("left")
	if err != nil || down == 0 || up == 0 {
		t.Fatalf("expected left button flags, got down=%d up=%d err=%v", down, up, err)
	}
	if _, _, err := mouseFlags("weird"); err == nil {
		t.Fatal("expected unsupported mouse button error")
	}

	command, err := desktopHumanClickCommand(10, 20, "right", true, 0, 0, -1, -1, 0)
	if err != nil {
		t.Fatalf("desktopHumanClickCommand: %v", err)
	}
	if !strings.Contains(command, `"double-clicked human"`) || !strings.Contains(command, "$doubleClick = $true") {
		t.Fatalf("unexpected human click command: %q", command)
	}
}

func TestDesktopNumericHelpers(t *testing.T) {
	if got := powerShellBool(true); got != "$true" {
		t.Fatalf("expected $true, got %q", got)
	}
	if !boolValue("TrUe") || boolValue("false") {
		t.Fatal("unexpected boolValue result")
	}
	if got, ok := numberInput("42"); !ok || got != 42 {
		t.Fatalf("expected string number parsing, got %d %v", got, ok)
	}
	if got := intNumberWithDefault("bad", 7); got != 7 {
		t.Fatalf("expected default fallback, got %d", got)
	}
	if encoded := utf16LE("AZ"); len(encoded) != 4 {
		t.Fatalf("expected UTF-16LE output, got %#v", encoded)
	}
}

func TestEnsureDesktopAllowedBehavior(t *testing.T) {
	err := ensureDesktopAllowed("desktop_click", BuiltinOptions{
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
	}, false)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("expected desktop tool to be allowed on Windows, got %v", err)
		}
	} else if err == nil {
		t.Fatal("expected non-Windows desktop execution to be denied")
	}

	if runtime.GOOS == "windows" {
		if err := ensureDesktopAllowed("desktop_click", BuiltinOptions{
			ExecutionMode:   "host-reviewed",
			PermissionLevel: "read-only",
		}, false); err == nil {
			t.Fatal("expected read-only desktop execution to be denied")
		}
	}
}

func TestDesktopToolsBuildCommands(t *testing.T) {
	originalOS := desktopHostOS
	originalRunner := desktopScriptRunner
	t.Cleanup(func() {
		desktopHostOS = originalOS
		desktopScriptRunner = originalRunner
	})

	desktopHostOS = "windows"
	var scripts []string
	desktopScriptRunner = func(_ context.Context, script string) (string, error) {
		scripts = append(scripts, script)
		return "ok", nil
	}

	opts := BuiltinOptions{
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
		WorkingDir:      t.TempDir(),
	}

	cases := []struct {
		name   string
		run    func() error
		expect string
	}{
		{"open", func() error {
			_, err := DesktopOpenTool(context.Background(), map[string]any{"target": "https://example.com", "kind": "url"}, opts)
			return err
		}, "opened url"},
		{"type", func() error {
			_, err := DesktopTypeTool(context.Background(), map[string]any{"text": "hello"}, opts)
			return err
		}, "typed"},
		{"type human", func() error {
			_, err := DesktopTypeHumanTool(context.Background(), map[string]any{"text": "hello", "submit": true}, opts)
			return err
		}, "typed human"},
		{"hotkey", func() error {
			_, err := DesktopHotkeyTool(context.Background(), map[string]any{"keys": []any{"ctrl", "s"}}, opts)
			return err
		}, "hotkey sent"},
		{"clipboard set", func() error {
			_, err := DesktopClipboardSetTool(context.Background(), map[string]any{"text": "clip"}, opts)
			return err
		}, "clipboard set"},
		{"clipboard get", func() error { _, err := DesktopClipboardGetTool(context.Background(), nil, opts); return err }, "Get-Clipboard"},
		{"paste", func() error {
			_, err := DesktopPasteTool(context.Background(), map[string]any{"text": "clip", "submit": true}, opts)
			return err
		}, "pasted"},
		{"click", func() error {
			_, err := DesktopClickTool(context.Background(), map[string]any{"x": 10, "y": 20, "button": "left"}, opts)
			return err
		}, "clicked"},
		{"move", func() error {
			_, err := DesktopMoveTool(context.Background(), map[string]any{"x": 10, "y": 20}, opts)
			return err
		}, "moved"},
		{"double click", func() error {
			_, err := DesktopDoubleClickTool(context.Background(), map[string]any{"x": 10, "y": 20, "button": "left"}, opts)
			return err
		}, "double-clicked"},
		{"scroll", func() error {
			_, err := DesktopScrollTool(context.Background(), map[string]any{"direction": "down"}, opts)
			return err
		}, "scrolled"},
		{"drag", func() error {
			_, err := DesktopDragTool(context.Background(), map[string]any{"x1": 1, "y1": 2, "x2": 3, "y2": 4, "button": "left"}, opts)
			return err
		}, "dragged"},
		{"wait", func() error {
			_, err := DesktopWaitTool(context.Background(), map[string]any{"wait_ms": 10}, opts)
			return err
		}, "waited"},
		{"focus", func() error {
			_, err := DesktopFocusWindowTool(context.Background(), map[string]any{"title": "Demo", "match": "contains"}, opts)
			return err
		}, "focused"},
		{"screenshot", func() error {
			_, err := DesktopScreenshotTool(context.Background(), map[string]any{"path": filepath.Join("shots", "screen.png")}, opts)
			return err
		}, "saved"},
	}

	for _, tc := range cases {
		before := len(scripts)
		if err := tc.run(); err != nil {
			t.Fatalf("%s returned error: %v", tc.name, err)
		}
		if len(scripts) != before+1 {
			t.Fatalf("%s did not invoke desktop script runner", tc.name)
		}
		if !strings.Contains(scripts[len(scripts)-1], tc.expect) {
			t.Fatalf("%s generated unexpected script: %q", tc.name, scripts[len(scripts)-1])
		}
	}
}

func TestDesktopToolValidationErrors(t *testing.T) {
	originalOS := desktopHostOS
	originalRunner := desktopScriptRunner
	t.Cleanup(func() {
		desktopHostOS = originalOS
		desktopScriptRunner = originalRunner
	})
	desktopHostOS = "windows"
	desktopScriptRunner = func(context.Context, string) (string, error) {
		return "", fmt.Errorf("should not be called")
	}

	opts := BuiltinOptions{
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
		WorkingDir:      t.TempDir(),
		ProtectedPaths:  []string{filepath.Join(t.TempDir(), "protected")},
	}

	if _, err := DesktopOpenTool(context.Background(), map[string]any{}, opts); err == nil {
		t.Fatal("expected missing target error")
	}
	if _, err := DesktopHotkeyTool(context.Background(), map[string]any{}, opts); err == nil {
		t.Fatal("expected missing keys error")
	}
	if _, err := DesktopScrollTool(context.Background(), map[string]any{"direction": "sideways"}, opts); err == nil {
		t.Fatal("expected invalid scroll direction error")
	}
	if _, err := DesktopScrollTool(context.Background(), map[string]any{"x": 1}, opts); err == nil {
		t.Fatal("expected mismatched x/y error")
	}
	if _, err := DesktopDragTool(context.Background(), map[string]any{"x1": 1}, opts); err == nil {
		t.Fatal("expected incomplete drag coordinates error")
	}
	if _, err := DesktopFocusWindowTool(context.Background(), map[string]any{"match": "bad", "title": "Demo"}, opts); err == nil {
		t.Fatal("expected invalid match mode error")
	}
	if _, err := DesktopScreenshotTool(context.Background(), map[string]any{}, opts); err == nil {
		t.Fatal("expected missing screenshot path error")
	}
}
