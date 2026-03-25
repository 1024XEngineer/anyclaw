package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestShellCommandWithShellAuto(t *testing.T) {
	cmd, err := shellCommandWithShell(context.Background(), "echo hello", "auto")
	if err != nil {
		t.Fatalf("shellCommandWithShell(auto) returned error: %v", err)
	}
	if len(cmd.Args) == 0 {
		t.Fatalf("expected command args")
	}
	if runtime.GOOS == "windows" && cmd.Args[0] != "cmd" {
		t.Fatalf("expected cmd on windows, got %q", cmd.Args[0])
	}
	if runtime.GOOS != "windows" && cmd.Args[0] != "sh" {
		t.Fatalf("expected sh on non-windows, got %q", cmd.Args[0])
	}
}

func TestShellCommandWithShellRejectsUnsupportedShell(t *testing.T) {
	if _, err := shellCommandWithShell(context.Background(), "echo hello", "fish"); err == nil {
		t.Fatal("expected unsupported shell error")
	}
}

func TestReviewCommandExecutionRequiresSandboxByDefault(t *testing.T) {
	err := reviewCommandExecution("echo hello", "", BuiltinOptions{ExecutionMode: "sandbox"})
	if err == nil {
		t.Fatal("expected sandbox-only mode to deny host execution without sandbox")
	}
}

func TestWriteFileToolWithPolicyBlocksProtectedPath(t *testing.T) {
	tempDir := t.TempDir()
	protected := filepath.Join(tempDir, "private")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := WriteFileToolWithPolicy(context.Background(), map[string]any{
		"path":    filepath.Join(protected, "secret.txt"),
		"content": "x",
	}, tempDir, BuiltinOptions{
		PermissionLevel: "full",
		ProtectedPaths:  []string{protected},
	})
	if err == nil {
		t.Fatal("expected protected path write to be denied")
	}
}

func TestReviewCommandExecutionBlocksProtectedPathReference(t *testing.T) {
	tempDir := t.TempDir()
	protected := filepath.Join(tempDir, "Documents")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := reviewCommandExecution("type "+filepath.Join(protected, "secret.txt"), "", BuiltinOptions{
		ExecutionMode: "host-reviewed",
		ProtectedPaths: []string{
			protected,
		},
	})
	if err == nil {
		t.Fatal("expected command referencing protected path to be denied")
	}
}

func TestEnsureDesktopAllowedRequiresHostReviewed(t *testing.T) {
	err := ensureDesktopAllowed("desktop_click", BuiltinOptions{ExecutionMode: "sandbox", PermissionLevel: "limited"}, false)
	if err == nil {
		t.Fatal("expected desktop tool to require host-reviewed mode")
	}
}
