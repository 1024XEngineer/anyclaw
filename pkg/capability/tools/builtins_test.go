package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestRegisterBuiltinsAddsOpenClawCompatibleAliases(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		WorkingDir:      workspace,
		PermissionLevel: "full",
		ExecutionMode:   "host-reviewed",
		Policy: NewPolicyEngine(PolicyOptions{
			WorkingDir:      workspace,
			PermissionLevel: "full",
		}),
	})

	readResult, err := registry.Call(context.Background(), "read", map[string]any{"path": target})
	if err != nil {
		t.Fatalf("read alias: %v", err)
	}
	if readResult != "hello" {
		t.Fatalf("read alias returned %q", readResult)
	}

	writeTarget := filepath.Join(workspace, "out.txt")
	if _, err := registry.Call(context.Background(), "write", map[string]any{"path": writeTarget, "content": "openclaw"}); err != nil {
		t.Fatalf("write alias: %v", err)
	}
	data, err := os.ReadFile(writeTarget)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "openclaw" {
		t.Fatalf("write alias wrote %q", data)
	}
}

func TestApplyPatchCompatToolAppliesUpdatePatch(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(target, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		WorkingDir:      workspace,
		PermissionLevel: "full",
		ExecutionMode:   "host-reviewed",
		Policy: NewPolicyEngine(PolicyOptions{
			WorkingDir:      workspace,
			PermissionLevel: "full",
		}),
	})

	result, err := registry.Call(context.Background(), "apply_patch", map[string]any{
		"input": "*** Begin Patch\n*** Update File: notes.txt\n@@\n alpha\n-beta\n+BETA\n gamma\n*** End Patch\n",
	})
	if err != nil {
		t.Fatalf("apply_patch: %v", err)
	}
	if !strings.Contains(result, "modified") || !strings.Contains(result, "notes.txt") {
		t.Fatalf("expected patch summary, got %q", result)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if string(data) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("unexpected patched content %q", data)
	}
}

func TestApplyPatchCompatToolUsesHunkLinePositionForDuplicateContext(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "notes.txt")
	original := strings.Join([]string{
		"first",
		"same",
		"target",
		"same",
		"middle",
		"same",
		"target",
		"same",
		"last",
		"",
	}, "\n")
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		WorkingDir:      workspace,
		PermissionLevel: "full",
		ExecutionMode:   "host-reviewed",
		Policy: NewPolicyEngine(PolicyOptions{
			WorkingDir:      workspace,
			PermissionLevel: "full",
		}),
	})

	_, err := registry.Call(context.Background(), "apply_patch", map[string]any{
		"input": "*** Begin Patch\n*** Update File: notes.txt\n@@ -6,3 +6,3 @@\n same\n-target\n+TARGET\n same\n*** End Patch\n",
	})
	if err != nil {
		t.Fatalf("apply_patch: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	want := strings.Join([]string{
		"first",
		"same",
		"target",
		"same",
		"middle",
		"same",
		"TARGET",
		"same",
		"last",
		"",
	}, "\n")
	if string(data) != want {
		t.Fatalf("patch should update second matching hunk, got %q", data)
	}
}

func TestApplyPatchCompatToolInsertsAfterZeroLengthOldRange(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		WorkingDir:      workspace,
		PermissionLevel: "full",
		ExecutionMode:   "host-reviewed",
		Policy: NewPolicyEngine(PolicyOptions{
			WorkingDir:      workspace,
			PermissionLevel: "full",
		}),
	})

	_, err := registry.Call(context.Background(), "apply_patch", map[string]any{
		"input": "*** Begin Patch\n*** Update File: notes.txt\n@@ -2,0 +3,1 @@\n+inserted\n*** End Patch\n",
	})
	if err != nil {
		t.Fatalf("apply_patch: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if string(data) != "one\ntwo\ninserted\nthree\n" {
		t.Fatalf("expected zero-length hunk to insert after old range line, got %q", data)
	}
}

func TestUpdatePlanCompatToolValidatesAndReturnsPlan(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{})

	result, err := registry.Call(context.Background(), "update_plan", map[string]any{
		"explanation": "continue with verification",
		"plan": []any{
			map[string]any{"step": "inspect", "status": "completed"},
			map[string]any{"step": "patch", "status": "in_progress"},
			map[string]any{"step": "test", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatalf("update_plan: %v", err)
	}
	if !strings.Contains(result, `"status":"updated"`) || !strings.Contains(result, `"in_progress"`) || !strings.Contains(result, "continue with verification") {
		t.Fatalf("expected update_plan JSON summary, got %q", result)
	}

	_, err = registry.Call(context.Background(), "update_plan", map[string]any{
		"plan": []any{
			map[string]any{"step": "one", "status": "in_progress"},
			map[string]any{"step": "two", "status": "in_progress"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "at most one in_progress") {
		t.Fatalf("expected in_progress validation error, got %v", err)
	}
}

func TestFetchURLToolHonorsEgressPolicy(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		Policy: NewPolicyEngine(PolicyOptions{
			AllowedEgressDomains: []string{"allowed.example"},
		}),
	})

	_, err := registry.Call(context.Background(), "fetch_url", map[string]any{
		"url": "https://blocked.example.invalid/private",
	})
	if err == nil || !strings.Contains(err.Error(), "egress denied") {
		t.Fatalf("expected egress policy denial, got %v", err)
	}
}

func TestWebSearchToolHonorsEgressPolicy(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		Policy: NewPolicyEngine(PolicyOptions{
			AllowedEgressDomains: []string{"allowed.example"},
		}),
	})

	_, err := registry.Call(context.Background(), "web_search", map[string]any{
		"query": "anyclaw",
	})
	if err == nil || !strings.Contains(err.Error(), "egress denied") {
		t.Fatalf("expected egress policy denial, got %v", err)
	}
}

func TestBrowserNavigationToolsHonorEgressPolicy(t *testing.T) {
	opts := BuiltinOptions{
		Policy: NewPolicyEngine(PolicyOptions{
			AllowedEgressDomains: []string{"allowed.example"},
		}),
	}

	_, err := BrowserNavigateToolWithPolicy(context.Background(), map[string]any{
		"url": "https://blocked.example.invalid",
	}, opts)
	if err == nil || !strings.Contains(err.Error(), "egress denied") {
		t.Fatalf("expected browser navigate egress denial, got %v", err)
	}

	_, err = BrowserTabNewToolWithPolicy(context.Background(), map[string]any{
		"url": "https://blocked.example.invalid",
	}, opts)
	if err == nil || !strings.Contains(err.Error(), "egress denied") {
		t.Fatalf("expected browser tab egress denial, got %v", err)
	}

	_, err = BrowserDownloadToolWithPolicy(context.Background(), map[string]any{
		"url":  "https://blocked.example.invalid/file.txt",
		"path": filepath.Join(t.TempDir(), "file.txt"),
	}, opts)
	if err == nil || !strings.Contains(err.Error(), "egress denied") {
		t.Fatalf("expected browser download egress denial, got %v", err)
	}
}

func TestFetchURLToolRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 2*1024*1024+1)))
	}))
	defer server.Close()

	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		Policy: NewPolicyEngine(PolicyOptions{}),
	})

	_, err := registry.Call(context.Background(), "fetch_url", map[string]any{"url": server.URL})
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("expected oversized response rejection, got %v", err)
	}
}

func TestSessionStatusCompatToolReturnsExecutionContext(t *testing.T) {
	workspace := t.TempDir()
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		WorkingDir:      workspace,
		PermissionLevel: "full",
		ExecutionMode:   "host-reviewed",
	})

	ctx := WithToolCaller(context.Background(), ToolCaller{
		Role:      ToolCallerRoleMainAgent,
		AgentName: "main",
	})
	ctx = WithSandboxScope(ctx, SandboxScope{SessionID: "sess-1", Channel: "web"})

	result, err := registry.Call(ctx, "session_status", map[string]any{})
	if err != nil {
		t.Fatalf("session_status: %v", err)
	}
	for _, want := range []string{
		`"session_id":"sess-1"`,
		`"channel":"web"`,
		`"caller_role":"main_agent"`,
		`"agent_name":"main"`,
		`"permission_level":"full"`,
		`"execution_mode":"host-reviewed"`,
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected session_status to contain %s, got %q", want, result)
		}
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

func TestReadFileToolWithPolicyBlocksOutsideWorkingDir(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "notes.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := ReadFileToolWithPolicy(context.Background(), map[string]any{
		"path": target,
	}, workspace, BuiltinOptions{
		WorkingDir: workspace,
		Policy:     NewPolicyEngine(PolicyOptions{WorkingDir: workspace}),
	})
	if err == nil {
		t.Fatal("expected read outside working directory to be denied")
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

func TestReviewCommandExecutionAllowsExplicitlyAllowedProtectedPathReference(t *testing.T) {
	tempDir := t.TempDir()
	protected := filepath.Join(tempDir, "Desktop")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := reviewCommandExecution("mkdir "+filepath.Join(protected, "hello"), "", BuiltinOptions{
		ExecutionMode:  "host-reviewed",
		ProtectedPaths: []string{protected},
		AllowedWritePaths: []string{
			protected,
		},
	})
	if err != nil {
		t.Fatalf("expected explicitly allowed protected path reference to pass review, got %v", err)
	}
}

func TestRunCommandToolWithPolicyBlocksOutsideWorkingDirCwd(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()

	_, err := RunCommandToolWithPolicy(context.Background(), map[string]any{
		"command": "echo hello",
		"cwd":     outsideDir,
	}, BuiltinOptions{
		WorkingDir:      workspace,
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "limited",
		Policy:          NewPolicyEngine(PolicyOptions{WorkingDir: workspace, PermissionLevel: "limited"}),
	})
	if err == nil {
		t.Fatal("expected command cwd outside working directory to be denied")
	}
}

func TestRunCommandToolWithPolicyNilSandboxUsesRequestedCwd(t *testing.T) {
	workspace := t.TempDir()
	requestedCwd := filepath.Join(workspace, "requested")
	if err := os.MkdirAll(requestedCwd, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	_, err := RunCommandToolWithPolicy(context.Background(), map[string]any{
		"command": "echo ok > marker.txt",
		"cwd":     requestedCwd,
	}, BuiltinOptions{
		WorkingDir:      workspace,
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
	})
	if err != nil {
		t.Fatalf("RunCommandToolWithPolicy: %v", err)
	}

	if _, err := os.Stat(filepath.Join(requestedCwd, "marker.txt")); err != nil {
		t.Fatalf("expected command to run in requested cwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected command to avoid working dir fallback, got %v", err)
	}
}

func TestRunCommandToolWithPolicyNilSandboxAllowsEmptyCwd(t *testing.T) {
	output, err := RunCommandToolWithPolicy(context.Background(), map[string]any{
		"command": "echo ok",
	}, BuiltinOptions{
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
	})
	if err != nil {
		t.Fatalf("RunCommandToolWithPolicy: %v", err)
	}
	if !strings.Contains(output, "ok") {
		t.Fatalf("expected command output, got %q", output)
	}
}

func TestEnsureDesktopAllowedRequiresHostReviewed(t *testing.T) {
	err := ensureDesktopAllowed("desktop_click", BuiltinOptions{ExecutionMode: "sandbox", PermissionLevel: "limited"}, false)
	if err == nil {
		t.Fatal("expected desktop tool to require host-reviewed mode")
	}
}

func TestMemoryToolsSearchAndGetDailyFiles(t *testing.T) {
	workspace := t.TempDir()
	memoryDir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "2026-03-29.md"), []byte("# Daily Memory 2026-03-29\n\nRelease checklist completed."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{WorkingDir: workspace})

	searchResult, err := registry.Call(context.Background(), "memory_search", map[string]any{"query": "checklist"})
	if err != nil {
		t.Fatalf("memory_search: %v", err)
	}
	if !strings.Contains(searchResult, "2026-03-29") {
		t.Fatalf("expected search result to mention date, got %q", searchResult)
	}

	getResult, err := registry.Call(context.Background(), "memory_get", map[string]any{"date": "2026-03-29"})
	if err != nil {
		t.Fatalf("memory_get: %v", err)
	}
	if !strings.Contains(getResult, "Release checklist completed.") {
		t.Fatalf("expected memory_get output, got %q", getResult)
	}
}
