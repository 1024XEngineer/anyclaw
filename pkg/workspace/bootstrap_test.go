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
