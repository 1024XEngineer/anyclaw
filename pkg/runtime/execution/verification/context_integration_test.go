package verification

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type stubDesktopTools struct {
	windowAppears  bool
	windowFocused  bool
	ocrText        string
	clipboard      string
	elementVisible bool
	screenshot     []byte
}

func (s stubDesktopTools) WindowAppears(title string) (bool, error) { return s.windowAppears, nil }
func (s stubDesktopTools) WindowFocused(title string) (bool, error) { return s.windowFocused, nil }
func (s stubDesktopTools) TextContains(x, y, width, height int, text string) (bool, error) {
	return s.ocrText != "" && contains(s.ocrText, text), nil
}
func (s stubDesktopTools) OCRText(x, y, width, height int) (string, error) { return s.ocrText, nil }
func (s stubDesktopTools) Clipboard() (string, error)                      { return s.clipboard, nil }
func (s stubDesktopTools) ElementVisible(selector string) (bool, error)    { return s.elementVisible, nil }
func (s stubDesktopTools) Screenshot() ([]byte, error)                     { return s.screenshot, nil }

func TestDefaultContextFileAndDesktopHelpers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := NewDefaultContext()
	ctx.SetDesktopTools(stubDesktopTools{
		windowAppears:  true,
		windowFocused:  true,
		ocrText:        "hello world",
		clipboard:      "copied text",
		elementVisible: true,
		screenshot:     []byte("png"),
	})

	if ok, err := ctx.FileExists(path); err != nil || !ok {
		t.Fatalf("FileExists failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ctx.FileContains(path, "hello"); err != nil || !ok {
		t.Fatalf("FileContains failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ctx.FileMatches(path, "hello"); err != nil || !ok {
		t.Fatalf("FileMatches failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ctx.WindowAppears("notepad"); err != nil || !ok {
		t.Fatalf("WindowAppears failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ctx.WindowFocused("notepad"); err != nil || !ok {
		t.Fatalf("WindowFocused failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ctx.TextContains(0, 0, 100, 100, "hello"); err != nil || !ok {
		t.Fatalf("TextContains failed: ok=%v err=%v", ok, err)
	}
	if got, err := ctx.OCRText(0, 0, 100, 100); err != nil || got != "hello world" {
		t.Fatalf("OCRText failed: got=%q err=%v", got, err)
	}
	if got, err := ctx.Clipboard(); err != nil || got != "copied text" {
		t.Fatalf("Clipboard failed: got=%q err=%v", got, err)
	}
	if ok, valid := ctx.ClipboardContains("copied"); !ok || !valid {
		t.Fatalf("ClipboardContains failed: ok=%v valid=%v", ok, valid)
	}
	if ok, err := ctx.ElementVisible("#app"); err != nil || !ok {
		t.Fatalf("ElementVisible failed: ok=%v err=%v", ok, err)
	}
	if shot, err := ctx.Screenshot(); err != nil || string(shot) != "png" {
		t.Fatalf("Screenshot failed: shot=%q err=%v", shot, err)
	}
	if _, err := ctx.CustomVerify("custom", nil); err == nil {
		t.Fatal("expected CustomVerify to be unimplemented")
	}
}

func TestDefaultContextNetworkRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	ctx := NewDefaultContext()
	status, err := ctx.NetworkRequest(server.URL)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("NetworkRequest failed: status=%d err=%v", status, err)
	}
}

func TestIntegrationContextAndHelpers(t *testing.T) {
	type ctxKey string
	const key ctxKey = "ctx"

	ic := &IntegrationContext{
		ctx: context.WithValue(context.Background(), key, "trace"),
		toolExec: func(ctx context.Context, toolName string, input map[string]any) (string, error) {
			if ctx.Value(key) != "trace" {
				t.Fatalf("expected context propagation for %s", toolName)
			}
			switch toolName {
			case "file_exists":
				return "exists", nil
			case "read_file":
				return "hello pattern content", nil
			case "desktop_list_windows":
				return "Calculator", nil
			case "desktop_get_foreground_window":
				return "Calculator", nil
			case "desktop_ocr":
				return "Visible text", nil
			case "clipboard_read":
				return "copied content", nil
			case "http_request":
				return "200 success", nil
			case "desktop_find_element":
				return "element found", nil
			case "desktop_screenshot":
				return "image-bytes", nil
			default:
				return "", fmt.Errorf("unexpected tool: %s", toolName)
			}
		},
	}

	if ok, err := ic.FileExists("a"); err != nil || !ok {
		t.Fatalf("FileExists failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ic.FileContains("a", "pattern"); err != nil || !ok {
		t.Fatalf("FileContains failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ic.FileMatches("a", "hello"); err != nil || !ok {
		t.Fatalf("FileMatches failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ic.WindowAppears("Calc"); err != nil || !ok {
		t.Fatalf("WindowAppears failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ic.WindowFocused("Calc"); err != nil || !ok {
		t.Fatalf("WindowFocused failed: ok=%v err=%v", ok, err)
	}
	if ok, err := ic.TextContains(0, 0, 1, 1, "Visible"); err != nil || !ok {
		t.Fatalf("TextContains failed: ok=%v err=%v", ok, err)
	}
	if got, err := ic.OCRText(0, 0, 1, 1); err != nil || got != "Visible text" {
		t.Fatalf("OCRText failed: got=%q err=%v", got, err)
	}
	if got, err := ic.Clipboard(); err != nil || got != "copied content" {
		t.Fatalf("Clipboard failed: got=%q err=%v", got, err)
	}
	if ok, valid := ic.ClipboardContains("copied"); !ok || !valid {
		t.Fatalf("ClipboardContains failed: ok=%v valid=%v", ok, valid)
	}
	if status, err := ic.NetworkRequest("https://example.com"); err != nil || status != 200 {
		t.Fatalf("NetworkRequest failed: status=%d err=%v", status, err)
	}
	if ok, err := ic.AppRunning("Calc"); err != nil || !ok {
		t.Fatalf("AppRunning failed: ok=%v err=%v", ok, err)
	}
	if state, err := ic.AppState("Calc"); err != nil || state["running"] != true {
		t.Fatalf("AppState failed: state=%#v err=%v", state, err)
	}
	if ok, err := ic.ElementVisible("#app"); err != nil || !ok {
		t.Fatalf("ElementVisible failed: ok=%v err=%v", ok, err)
	}
	if shot, err := ic.Screenshot(); err != nil || string(shot) != "image-bytes" {
		t.Fatalf("Screenshot failed: shot=%q err=%v", shot, err)
	}
	if _, err := ic.CustomVerify("custom", nil); err == nil {
		t.Fatal("expected CustomVerify to be unimplemented")
	}
}

func TestIntegrationHelperFunctions(t *testing.T) {
	exec := NewIntegrationExecutor(func(ctx context.Context, toolName string, input map[string]any) (string, error) {
		return "", nil
	})

	mappings := map[string]VerificationType{
		"desktop_verify_text":  VerificationTypeTextContains,
		"desktop_wait_text":    VerificationTypeTextContains,
		"desktop_ocr":          VerificationTypeOCRContains,
		"desktop_screenshot":   VerificationTypeScreenshot,
		"file_exists":          VerificationTypeFileExists,
		"clipboard_read":       VerificationTypeClipboard,
		"http_request":         VerificationTypeNetwork,
		"desktop_list_windows": VerificationTypeWindowAppears,
		"desktop_find_element": VerificationTypeElementVisible,
		"something-else":       VerificationTypeCustom,
	}
	for tool, want := range mappings {
		if got := exec.mapToolToVerificationType(tool); got != want {
			t.Fatalf("mapToolToVerificationType(%q) = %q, want %q", tool, got, want)
		}
	}

	merged := mergeParams(map[string]any{"a": 1, "shared": "target"}, map[string]any{"b": 2, "shared": "input"})
	if merged["a"] != 1 || merged["b"] != 2 || merged["shared"] != "input" {
		t.Fatalf("unexpected merge params result: %#v", merged)
	}

	if !contains("abcdef", "cde") {
		t.Fatal("expected contains to match substring")
	}
	if contains("abc", "") {
		t.Fatal("expected contains to reject empty substring")
	}
	if !containsStr("abcdef", "def") {
		t.Fatal("expected containsStr to find substring")
	}
	if !match("bc", "abcdef") {
		t.Fatal("expected match to find substring")
	}
	if !match("", "abcdef") {
		t.Fatal("expected empty pattern to match")
	}
}
