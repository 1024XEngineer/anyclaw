package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAnyClawCLIRoutesClawCommand(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"claw"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI claw: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw claw commands:") {
		t.Fatalf("expected claw usage output, got %q", stdout)
	}
}

func TestRunClawStatusLoadsBridgeRoot(t *testing.T) {
	root := writeClawBridgeFixture(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runClawCommand([]string{"status", "--root", root})
	})
	if err != nil {
		t.Fatalf("runClawCommand status: %v", err)
	}
	for _, want := range []string{
		"claw-code-main bridge: available",
		"commands: 2 snapshot entries",
		"tools: 1 snapshot entries",
		"subsystems: 1 mirrored areas",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunClawLookupPrintsJSON(t *testing.T) {
	root := writeClawBridgeFixture(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runClawCommand([]string{"lookup", "--root", root, "--section", "commands", "--family", "agents"})
	})
	if err != nil {
		t.Fatalf("runClawCommand lookup: %v", err)
	}
	if !strings.Contains(stdout, `"section": "commands"`) || !strings.Contains(stdout, `"name": "agents"`) {
		t.Fatalf("expected command family JSON, got %q", stdout)
	}
}

func writeClawBridgeFixture(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "claw-code-main")
	refDir := filepath.Join(root, "src", "reference_data")
	subsystemsDir := filepath.Join(refDir, "subsystems")
	if err := os.MkdirAll(subsystemsDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}

	writeClawJSON(t, filepath.Join(refDir, "commands_snapshot.json"), []map[string]string{
		{"name": "agents", "source_hint": "commands/agents/index.ts"},
		{"name": "tasks", "source_hint": "commands/tasks/index.ts"},
	})
	writeClawJSON(t, filepath.Join(refDir, "tools_snapshot.json"), []map[string]string{
		{"name": "AgentTool", "source_hint": "tools/AgentTool/index.ts"},
	})
	writeClawJSON(t, filepath.Join(subsystemsDir, "cli.json"), map[string]any{
		"archive_name": "cli",
		"module_count": 7,
		"sample_files": []string{"cli/handlers/agents.ts"},
	})

	return root
}

func writeClawJSON(t *testing.T, path string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}
