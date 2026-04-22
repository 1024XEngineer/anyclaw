package tools

import (
	"context"
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

func TestWebSearchToolWithPolicyRejectsDisallowedEgress(t *testing.T) {
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:           t.TempDir(),
		AllowedEgressDomains: []string{"example.com"},
	})

	_, err := WebSearchToolWithPolicy(context.Background(), map[string]any{
		"query": "golang",
	}, BuiltinOptions{Policy: policy})
	if err == nil {
		t.Fatal("expected web search to be denied when search provider host is not allowlisted")
	}
}

func TestFetchURLToolWithPolicyRejectsDisallowedEgress(t *testing.T) {
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:           t.TempDir(),
		AllowedEgressDomains: []string{"example.com"},
	})

	_, err := FetchURLToolWithPolicy(context.Background(), map[string]any{
		"url": "https://openai.com",
	}, BuiltinOptions{Policy: policy})
	if err == nil {
		t.Fatal("expected fetch_url to be denied when target host is not allowlisted")
	}
}

func TestRegisterToolDoesNotCacheByDefault(t *testing.T) {
	registry := NewRegistry()
	callCount := 0
	registry.RegisterTool("side_effect", "demo", map[string]any{}, func(_ context.Context, _ map[string]any) (string, error) {
		callCount++
		return strings.Repeat("x", callCount), nil
	})

	first, err := registry.Call(context.Background(), "side_effect", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("first Call: %v", err)
	}
	second, err := registry.Call(context.Background(), "side_effect", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("second Call: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected handler to run twice without default caching, got %d", callCount)
	}
	if first == second {
		t.Fatalf("expected different results from repeated side-effecting calls, got %q and %q", first, second)
	}
}

func TestQMDInsertGeneratesUniqueIDs(t *testing.T) {
	client := &stubQMDClient{}
	input := map[string]any{
		"data": map[string]any{
			"name": "demo",
		},
	}

	if _, err := qmdInsert(context.Background(), client, "tasks", input); err != nil {
		t.Fatalf("first qmdInsert: %v", err)
	}
	if _, err := qmdInsert(context.Background(), client, "tasks", input); err != nil {
		t.Fatalf("second qmdInsert: %v", err)
	}

	if len(client.insertedIDs) != 2 {
		t.Fatalf("expected two inserted IDs, got %d", len(client.insertedIDs))
	}
	if client.insertedIDs[0] == client.insertedIDs[1] {
		t.Fatalf("expected generated QMD IDs to differ, got %q", client.insertedIDs[0])
	}
}

func TestBuiltinPathAndCommandHelpers(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "secret")
	allowed := filepath.Join(protected, "public")

	if err := validateProtectedPath(filepath.Join(protected, "a.txt"), []string{protected}); err == nil {
		t.Fatal("expected protected path validation failure")
	}
	if err := validateCommandAgainstProtectedPaths("cat "+filepath.Join(protected, "a.txt"), []string{protected}, nil); err == nil {
		t.Fatal("expected command to be rejected for protected path reference")
	}
	if !isProtectedPathExplicitlyAllowed(protected, []string{allowed}) {
		t.Fatal("expected nested allowed path to mark protected path as explicitly allowed")
	}
	if merged := mergePathLists(workspace, []string{allowed}, []string{""}); len(merged) < 2 {
		t.Fatalf("expected merged path list, got %#v", merged)
	}
	if !pathWithin(filepath.Join(workspace, "sub"), workspace) {
		t.Fatal("expected child path to be within workspace")
	}
	if normalizeCommandForCompare(`C:\Users\Test`) != "c:/users/test" {
		t.Fatal("expected normalized command slashes")
	}
	if tokens := protectedPathTokens(filepath.Join(protected, "Documents")); len(tokens) == 0 {
		t.Fatal("expected protected path tokens to be generated")
	}
}

func TestBuiltinGeneralHelpers(t *testing.T) {
	if got := resolvePath("notes.txt", filepath.Join("tmp", "work")); !strings.HasSuffix(got, filepath.Join("tmp", "work", "notes.txt")) {
		t.Fatalf("unexpected resolved path %q", got)
	}
	if !isImageFile("demo.PNG") {
		t.Fatal("expected image extension detection")
	}
	if firstNonEmptyCommandCwd("", " ", "here") != "here" {
		t.Fatal("expected first non-empty cwd")
	}
	if normalizeShellName(" PwSh ") != "pwsh" {
		t.Fatal("expected normalized shell name")
	}
	if !isDangerousCommand("rm -rf /tmp/demo", []string{"rm -rf"}) {
		t.Fatal("expected dangerous command detection")
	}
}

func TestEnsureWriteAllowedModes(t *testing.T) {
	workspace := t.TempDir()
	if err := ensureWriteAllowed(filepath.Join(workspace, "file.txt"), workspace, "limited"); err != nil {
		t.Fatalf("expected limited write inside workspace, got %v", err)
	}
	if err := ensureWriteAllowed(filepath.Join(t.TempDir(), "file.txt"), workspace, "limited"); err == nil {
		t.Fatal("expected limited write outside workspace to be denied")
	}
	if err := ensureWriteAllowed(filepath.Join(workspace, "file.txt"), workspace, "read-only"); err == nil {
		t.Fatal("expected read-only write to be denied")
	}
}

func TestGetTimeToolAndReviewCommandExecution(t *testing.T) {
	result, err := GetTimeTool(context.Background(), map[string]any{"format": "2006"})
	if err != nil || len(strings.TrimSpace(result)) != 4 {
		t.Fatalf("GetTimeTool returned %q, %v", result, err)
	}

	workspace := t.TempDir()
	protected := filepath.Join(workspace, "private")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	err = reviewCommandExecution("cat "+filepath.Join(protected, "x.txt"), "", BuiltinOptions{
		ExecutionMode: "host-reviewed",
		ProtectedPaths: []string{
			protected,
		},
	})
	if err == nil {
		t.Fatal("expected protected path command review failure")
	}
}

func TestFileBuiltinTools(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "docs", "note.txt")

	if _, err := WriteFileTool(context.Background(), map[string]any{
		"path":    target,
		"content": "hello world",
	}); err != nil {
		t.Fatalf("WriteFileTool: %v", err)
	}

	read, err := ReadFileTool(context.Background(), map[string]any{"path": target})
	if err != nil {
		t.Fatalf("ReadFileTool: %v", err)
	}
	if read != "hello world" {
		t.Fatalf("unexpected file contents: %q", read)
	}

	dirJSON, err := ListDirectoryToolWithCwd(context.Background(), map[string]any{"path": filepath.Dir(target)}, workspace)
	if err != nil {
		t.Fatalf("ListDirectoryToolWithCwd: %v", err)
	}
	if !strings.Contains(dirJSON, "note.txt") {
		t.Fatalf("expected directory listing to include note.txt, got %q", dirJSON)
	}

	searchJSON, err := SearchFilesToolWithCwd(context.Background(), map[string]any{
		"path":    workspace,
		"pattern": "*.txt",
	}, workspace)
	if err != nil {
		t.Fatalf("SearchFilesToolWithCwd: %v", err)
	}
	if !strings.Contains(searchJSON, "note.txt") {
		t.Fatalf("expected search results to include note.txt, got %q", searchJSON)
	}

	rootList, err := ListDirectoryTool(context.Background(), map[string]any{"path": workspace})
	if err != nil {
		t.Fatalf("ListDirectoryTool: %v", err)
	}
	if !strings.Contains(rootList, "docs/") {
		t.Fatalf("expected root listing to include docs directory, got %q", rootList)
	}

	searchViaWrapper, err := SearchFilesTool(context.Background(), map[string]any{
		"path":    workspace,
		"pattern": "*.txt",
	})
	if err != nil {
		t.Fatalf("SearchFilesTool: %v", err)
	}
	if !strings.Contains(searchViaWrapper, "note.txt") {
		t.Fatalf("expected wrapper search results to include note.txt, got %q", searchViaWrapper)
	}
}

func TestRunCommandAndShellHelpers(t *testing.T) {
	output, err := RunCommandToolWithPolicy(context.Background(), map[string]any{"command": "echo hello"}, BuiltinOptions{
		ExecutionMode: "host-reviewed",
	})
	if err != nil {
		t.Fatalf("RunCommandToolWithPolicy: %v", err)
	}
	if !strings.Contains(strings.ToLower(output), "hello") {
		t.Fatalf("expected command output to contain hello, got %q", output)
	}

	cmd := shellCommand(context.Background(), "echo hello")
	if cmd == nil {
		t.Fatal("expected shellCommand to return a command")
	}
	if exe, err := findShellExecutable("go"); err != nil || !strings.HasSuffix(strings.ToLower(exe), "go.exe") && !strings.HasSuffix(strings.ToLower(exe), "/go") {
		t.Fatalf("findShellExecutable returned %q, %v", exe, err)
	}
}

func TestBuiltinWrapperAndPolicyHelpers(t *testing.T) {
	workspace := t.TempDir()
	filePath := filepath.Join(workspace, "a.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:       workspace,
		AllowedReadPaths: []string{workspace},
	})

	if got, err := ReadFileToolWithPolicy(context.Background(), map[string]any{"path": filePath}, workspace, BuiltinOptions{Policy: policy}); err != nil || got != "hello" {
		t.Fatalf("ReadFileToolWithPolicy returned %q, %v", got, err)
	}

	list, err := ListDirectoryToolWithPolicy(context.Background(), map[string]any{"path": workspace}, workspace, BuiltinOptions{Policy: policy})
	if err != nil || !strings.Contains(list, "a.txt") {
		t.Fatalf("ListDirectoryToolWithPolicy returned %q, %v", list, err)
	}

	search, err := SearchFilesToolWithPolicy(context.Background(), map[string]any{"path": workspace, "pattern": "*.txt"}, workspace, BuiltinOptions{Policy: policy})
	if err != nil || !strings.Contains(search, "a.txt") {
		t.Fatalf("SearchFilesToolWithPolicy returned %q, %v", search, err)
	}

	if _, err := RunCommandTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected RunCommandTool missing command error")
	}
	if _, err := WebSearchTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected WebSearchTool missing query error")
	}
	if _, err := FetchURLTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected FetchURLTool missing url error")
	}
}

func TestReadFileToolWithCwdRejectsImageInput(t *testing.T) {
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "shot.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	if _, err := ReadFileToolWithCwd(context.Background(), map[string]any{"path": imagePath}, workspace); err == nil {
		t.Fatal("expected image input to be rejected")
	}
}

func TestBuiltinsAdditionalBranches(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "protected")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}

	if _, err := WriteFileToolWithPolicy(context.Background(), map[string]any{
		"path":    filepath.Join(workspace, "ok.txt"),
		"content": "ok",
	}, workspace, BuiltinOptions{
		PermissionLevel: "full",
		Policy:          NewPolicyEngine(PolicyOptions{WorkingDir: workspace, AllowedWritePaths: []string{workspace}}),
	}); err != nil {
		t.Fatalf("WriteFileToolWithPolicy allow path: %v", err)
	}

	emptyDir := filepath.Join(workspace, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}
	if got, err := ListDirectoryToolWithCwd(context.Background(), map[string]any{"path": emptyDir}, workspace); err != nil || got != "Empty directory" {
		t.Fatalf("ListDirectoryToolWithCwd empty returned %q, %v", got, err)
	}
	if got, err := SearchFilesToolWithCwd(context.Background(), map[string]any{"path": workspace, "pattern": "*.missing"}, workspace); err != nil || got != "No matches found" {
		t.Fatalf("SearchFilesToolWithCwd no matches returned %q, %v", got, err)
	}
	if _, err := ReadFileToolWithCwd(context.Background(), map[string]any{}, workspace); err == nil {
		t.Fatal("expected missing path error from ReadFileToolWithCwd")
	}
	if _, err := WriteFileToolWithCwd(context.Background(), map[string]any{"path": "x.txt"}, workspace, "full"); err == nil {
		t.Fatal("expected missing content error from WriteFileToolWithCwd")
	}

	if _, err := RunCommandToolWithPolicy(context.Background(), map[string]any{"command": "echo hi"}, BuiltinOptions{
		PermissionLevel: "read-only",
	}); err == nil {
		t.Fatal("expected read-only command execution to be denied")
	}

	cancelled := false
	if _, err := RunCommandToolWithPolicy(context.Background(), map[string]any{"command": "rm -rf tmp"}, BuiltinOptions{
		ExecutionMode:     "host-reviewed",
		DangerousPatterns: []string{"rm -rf"},
		ConfirmDangerousCommand: func(string) bool {
			cancelled = true
			return false
		},
	}); err == nil || !cancelled {
		t.Fatal("expected dangerous command confirmation to cancel execution")
	}
}

type stubQMDClient struct {
	insertedIDs []string
}

func (s *stubQMDClient) CreateTable(context.Context, string, []string) error {
	return nil
}

func (s *stubQMDClient) Insert(_ context.Context, _ string, record map[string]any) error {
	if id, _ := record["id"].(string); id != "" {
		s.insertedIDs = append(s.insertedIDs, id)
	}
	return nil
}

func (s *stubQMDClient) Get(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}

func (s *stubQMDClient) Update(context.Context, string, map[string]any) error {
	return nil
}

func (s *stubQMDClient) Delete(context.Context, string, string) error {
	return nil
}

func (s *stubQMDClient) List(context.Context, string, int) ([]map[string]any, error) {
	return nil, nil
}

func (s *stubQMDClient) Query(context.Context, string, string, any, int) ([]map[string]any, error) {
	return nil, nil
}

func (s *stubQMDClient) ListTables(context.Context) ([]TableStat, error) {
	return nil, nil
}

func (s *stubQMDClient) Count(context.Context, string) (int, error) {
	return 0, nil
}
