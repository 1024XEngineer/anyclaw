package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestSandboxScopeHelpers(t *testing.T) {
	scope := SandboxScope{SessionID: "s-1", Channel: "room"}
	ctx := WithSandboxScope(context.Background(), scope)

	if got := sandboxScopeFromContext(ctx); got != scope {
		t.Fatalf("unexpected sandbox scope: %#v", got)
	}
	if got := sandboxScopeFromContext(nil); got != (SandboxScope{}) {
		t.Fatalf("expected empty scope for nil context, got %#v", got)
	}
}

func TestSandboxManagerResolveExecutionDisabled(t *testing.T) {
	manager := NewSandboxManager(config.SandboxConfig{Enabled: false}, "C:/workspace")
	if manager.Enabled() {
		t.Fatal("expected disabled sandbox manager to report false")
	}
	cwd, factory, err := manager.ResolveExecution(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveExecution disabled: %v", err)
	}
	if cwd != "C:/workspace" {
		t.Fatalf("expected working dir fallback, got %q", cwd)
	}
	if factory != nil {
		t.Fatalf("expected no command factory when sandbox disabled")
	}
}

func TestSandboxManagerResolveExecutionLocal(t *testing.T) {
	baseDir := t.TempDir()
	manager := NewSandboxManager(config.SandboxConfig{
		Enabled: true,
		Backend: "local",
		BaseDir: baseDir,
	}, t.TempDir())
	ctx := WithSandboxScope(context.Background(), SandboxScope{
		SessionID: "sess:01",
		Channel:   "my/channel",
	})

	root, factory, err := manager.ResolveExecution(ctx, "")
	if err != nil {
		t.Fatalf("ResolveExecution local: %v", err)
	}
	if !manager.Enabled() {
		t.Fatal("expected enabled sandbox manager to report true")
	}
	if factory != nil {
		t.Fatalf("expected nil factory for local sandbox")
	}
	if !strings.HasPrefix(root, baseDir) {
		t.Fatalf("expected local sandbox under base dir, got %q", root)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("expected local sandbox directory to exist: %v", err)
	}
}

func TestSandboxManagerResolveExecutionUnsupportedBackend(t *testing.T) {
	manager := NewSandboxManager(config.SandboxConfig{
		Enabled: true,
		Backend: "weird",
	}, t.TempDir())

	if _, _, err := manager.ResolveExecution(context.Background(), ""); err == nil {
		t.Fatal("expected unsupported sandbox backend error")
	}
}

func TestSanitizeSandboxKey(t *testing.T) {
	key := sanitizeSandboxKey(SandboxScope{
		SessionID: "ABC:123",
		Channel:   "Room / Alpha",
	})
	if strings.ContainsAny(key, "/\\: ") {
		t.Fatalf("expected sanitized sandbox key, got %q", key)
	}
	if sanitizeSandboxKey(SandboxScope{}) != "default" {
		t.Fatal("expected default sandbox key when scope is empty")
	}
}

func TestEnsureLocalSandboxUsesDefaultBaseDir(t *testing.T) {
	workingDir := t.TempDir()
	manager := NewSandboxManager(config.SandboxConfig{
		Enabled: true,
		Backend: "local",
	}, workingDir)

	root, err := manager.ensureLocalSandbox("demo")
	if err != nil {
		t.Fatalf("ensureLocalSandbox: %v", err)
	}
	expectedPrefix := filepath.Join(workingDir, ".anyclaw", "sandboxes")
	if !strings.HasPrefix(root, expectedPrefix) {
		t.Fatalf("expected default sandbox root under %q, got %q", expectedPrefix, root)
	}
}

func TestSandboxManagerResolveExecutionDocker(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("docker sandbox stub uses a Windows batch shim in this environment")
	}

	toolDir := t.TempDir()
	dockerShim := filepath.Join(toolDir, "docker.cmd")
	script := "@echo off\r\n" +
		"if \"%1\"==\"inspect\" exit /b 1\r\n" +
		"if \"%1\"==\"run\" (\r\n" +
		"  echo started\r\n" +
		"  exit /b 0\r\n" +
		")\r\n" +
		"if \"%1\"==\"exec\" (\r\n" +
		"  echo exec-ok\r\n" +
		"  exit /b 0\r\n" +
		")\r\n" +
		"exit /b 0\r\n"
	if err := os.WriteFile(dockerShim, []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", toolDir+";"+originalPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	manager := NewSandboxManager(config.SandboxConfig{
		Enabled: true,
		Backend: "docker",
	}, t.TempDir())
	ctx := WithSandboxScope(context.Background(), SandboxScope{SessionID: "sess", Channel: "room"})

	cwd, factory, err := manager.ResolveExecution(ctx, "")
	if err != nil {
		t.Fatalf("ResolveExecution docker: %v", err)
	}
	if cwd != "/workspace" || factory == nil {
		t.Fatalf("expected docker cwd/factory, got cwd=%q factory=%v", cwd, factory != nil)
	}

	cmd, err := factory(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("docker factory: %v", err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker exec shim failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(output)), "exec-ok") {
		t.Fatalf("unexpected docker exec shim output: %q", string(output))
	}
}
