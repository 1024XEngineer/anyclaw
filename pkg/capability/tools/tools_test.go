package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterBuiltinsRegistersExpectedTools(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{WorkingDir: t.TempDir()})

	expected := []string{
		"read_file",
		"write_file",
		"memory_search",
		"web_search",
		"fetch_url",
		"desktop_open",
		"desktop_screenshot",
	}
	for _, name := range expected {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("expected builtin tool %q to be registered", name)
		}
	}
}

func TestRegisterBuiltinsClosuresAndAudit(t *testing.T) {
	originalOS := desktopHostOS
	originalRunner := desktopScriptRunner
	t.Cleanup(func() {
		desktopHostOS = originalOS
		desktopScriptRunner = originalRunner
	})
	desktopHostOS = "windows"
	desktopScriptRunner = func(_ context.Context, script string) (string, error) {
		return script, nil
	}

	workspace := t.TempDir()
	target := filepath.Join(workspace, "note.txt")
	var auditCalls []string
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		WorkingDir:      workspace,
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
		AuditLogger: auditLoggerFunc(func(toolName string, _ map[string]any, _ string, _ error) {
			auditCalls = append(auditCalls, toolName)
		}),
	})

	if _, err := registry.Call(context.Background(), "write_file", map[string]any{"path": target, "content": "demo"}); err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if got, err := registry.Call(context.Background(), "read_file", map[string]any{"path": target}); err != nil || got != "demo" {
		t.Fatalf("read_file returned %q, %v", got, err)
	}
	if _, err := registry.Call(context.Background(), "memory_get", map[string]any{"date": "missing"}); err == nil {
		t.Fatal("expected memory_get closure to propagate lookup error")
	}
	if _, err := registry.Call(context.Background(), "web_search", map[string]any{}); err == nil {
		t.Fatal("expected web_search closure validation error")
	}
	if got, err := registry.Call(context.Background(), "desktop_wait", map[string]any{"wait_ms": 5}); err != nil || !strings.Contains(got, "waited") {
		t.Fatalf("desktop_wait returned %q, %v", got, err)
	}
	if len(auditCalls) == 0 {
		t.Fatal("expected audit logger to be invoked")
	}
}

func TestAuditCallWithoutLogger(t *testing.T) {
	wrapped := auditCall(BuiltinOptions{}, "noop", map[string]any{"x": 1}, func(context.Context, map[string]any) (string, error) {
		return "ok", nil
	})
	got, err := wrapped(context.Background(), nil)
	if err != nil || got != "ok" {
		t.Fatalf("auditCall returned %q, %v", got, err)
	}
}

type auditLoggerFunc func(toolName string, input map[string]any, output string, err error)

func (f auditLoggerFunc) LogTool(toolName string, input map[string]any, output string, err error) {
	f(toolName, input, output, err)
}
