package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBootstrapCreatesOpenClawStyleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureBootstrap(dir, BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Execution helper",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	for _, name := range []string{"AGENTS.md", "SOUL.md", "TOOLS.md", "IDENTITY.md", "USER.md", "HEARTBEAT.md", "BOOTSTRAP.md", "MEMORY.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Fatalf("expected %s to be non-empty", name)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "memory")); err != nil || !info.IsDir() {
		t.Fatalf("expected memory directory: %v", err)
	}

	agentsData, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md): %v", err)
	}
	if !strings.Contains(string(agentsData), "inspect -> act -> inspect -> adapt -> verify") {
		t.Fatalf("expected AGENTS.md to describe the execution loop, got %q", string(agentsData))
	}

	toolsData, err := os.ReadFile(filepath.Join(dir, "TOOLS.md"))
	if err != nil {
		t.Fatalf("ReadFile(TOOLS.md): %v", err)
	}
	if !strings.Contains(string(toolsData), "observe the current world state") {
		t.Fatalf("expected TOOLS.md to describe current-state observation, got %q", string(toolsData))
	}
}

func TestEnsureBootstrapDoesNotOverwriteExistingFiles(t *testing.T) {
	dir := t.TempDir()
	custom := "# IDENTITY\n\nKeep this value."
	if err := os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte(custom), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := EnsureBootstrap(dir, BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Execution helper",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != custom {
		t.Fatalf("expected existing IDENTITY.md to be preserved, got %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(dir, "BOOTSTRAP.md")); err == nil {
		t.Fatal("did not expect BOOTSTRAP.md to be created when workspace already had bootstrap files")
	}
}

func TestEnsureBootstrapRespectsLowercaseMemoryFallback(t *testing.T) {
	dir := t.TempDir()
	custom := "# memory\n\nKeep this value."
	if err := os.WriteFile(filepath.Join(dir, "memory.md"), []byte(custom), 0o644); err != nil {
		t.Fatalf("WriteFile(memory.md): %v", err)
	}

	if err := EnsureBootstrap(dir, BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Execution helper",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "BOOTSTRAP.md")); err == nil {
		t.Fatal("did not expect BOOTSTRAP.md when lowercase memory.md already exists")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	memoryFiles := 0
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "memory.md") {
			memoryFiles++
		}
	}
	if memoryFiles != 1 {
		t.Fatalf("expected exactly one memory file, got %d", memoryFiles)
	}

	data, err := os.ReadFile(filepath.Join(dir, "memory.md"))
	if err != nil {
		t.Fatalf("ReadFile(memory.md): %v", err)
	}
	if string(data) != custom {
		t.Fatalf("expected memory.md to be preserved, got %q", string(data))
	}
}

func TestEnsureBootstrapAutoCompletesConfiguredWorkspaceAndPreservesExistingAnswers(t *testing.T) {
	dir := t.TempDir()
	opts := BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Execution helper",
	}
	if err := EnsureBootstrap(dir, opts); err != nil {
		t.Fatalf("EnsureBootstrap(initial): %v", err)
	}

	if _, err := AdvanceBootstrapRitual(dir, "", BootstrapRitualOptions{
		AgentName:        opts.AgentName,
		AgentDescription: opts.AgentDescription,
	}); err != nil {
		t.Fatalf("AdvanceBootstrapRitual(start): %v", err)
	}
	if _, err := AdvanceBootstrapRitual(dir, "Use Chinese by default.", BootstrapRitualOptions{
		AgentName:        opts.AgentName,
		AgentDescription: opts.AgentDescription,
	}); err != nil {
		t.Fatalf("AdvanceBootstrapRitual(answer): %v", err)
	}

	opts.UserProfile = "Default language: zh-CN"
	opts.WorkspaceFocus = "Help with coding in this workspace."
	opts.AssistantStyle = "Be concise and proactive."
	if err := EnsureBootstrap(dir, opts); err != nil {
		t.Fatalf("EnsureBootstrap(seed): %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected BOOTSTRAP.md to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, bootstrapStateFilename)); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap state to be removed, stat err=%v", err)
	}

	userData, err := os.ReadFile(filepath.Join(dir, "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md): %v", err)
	}
	userText := string(userData)
	if !strings.Contains(userText, "Use Chinese by default.") {
		t.Fatalf("expected USER.md to preserve existing answer, got %q", userText)
	}
	if !strings.Contains(userText, "<!-- anyclaw:bootstrap:start -->") {
		t.Fatalf("expected USER.md to contain managed bootstrap block, got %q", userText)
	}

	identityData, err := os.ReadFile(filepath.Join(dir, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("ReadFile(IDENTITY.md): %v", err)
	}
	identityText := string(identityData)
	if !strings.Contains(identityText, "Help with coding in this workspace.") {
		t.Fatalf("expected IDENTITY.md to include workspace focus, got %q", identityText)
	}
	if !strings.Contains(identityText, "Be concise and proactive.") {
		t.Fatalf("expected IDENTITY.md to include assistant style, got %q", identityText)
	}
}

func TestHasInjectedMemoryFileIgnoresMissingPlaceholder(t *testing.T) {
	tests := []struct {
		name  string
		files []BootstrapFile
		want  bool
	}{
		{
			name: "missing placeholder is ignored",
			files: []BootstrapFile{
				{Name: "MEMORY.md", Missing: true},
			},
			want: false,
		},
		{
			name: "existing uppercase memory file is detected",
			files: []BootstrapFile{
				{Name: "MEMORY.md"},
			},
			want: true,
		},
		{
			name: "existing lowercase memory file is detected",
			files: []BootstrapFile{
				{Name: "memory.md"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasInjectedMemoryFile(tt.files)
			if got != tt.want {
				t.Fatalf("HasInjectedMemoryFile() = %v, want %v", got, tt.want)
			}
		})
	}
}
