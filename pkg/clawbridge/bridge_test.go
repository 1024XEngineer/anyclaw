package clawbridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAutoDiscoversSiblingClawCodeMain(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "claw-code-main")
	if err := writeFixtureBridge(root); err != nil {
		t.Fatalf("writeFixtureBridge: %v", err)
	}

	start := filepath.Join(workspace, "anyclaw", "workflows", "default")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	summary, err := LoadAuto(start)
	if err != nil {
		t.Fatalf("LoadAuto: %v", err)
	}
	if summary.Root != root {
		t.Fatalf("expected root %q, got %q", root, summary.Root)
	}
	if summary.CommandsCount != 5 || summary.ToolsCount != 4 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
	if len(summary.CommandFamily) == 0 || summary.CommandFamily[0].Name != "agents" {
		t.Fatalf("expected grouped command families, got %+v", summary.CommandFamily)
	}
	if len(summary.ToolFamily) == 0 || summary.ToolFamily[0].Name != "AgentTool" {
		t.Fatalf("expected grouped tool families, got %+v", summary.ToolFamily)
	}
}

func TestDiscoverRootPrefersEnvOverride(t *testing.T) {
	root := filepath.Join(t.TempDir(), "claw-code-main")
	if err := writeFixtureBridge(root); err != nil {
		t.Fatalf("writeFixtureBridge: %v", err)
	}

	t.Setenv(EnvRoot, root)
	if discovered, ok := DiscoverRoot(""); !ok || discovered != root {
		t.Fatalf("expected env root %q, got %q ok=%v", root, discovered, ok)
	}
}

func TestLookupAndRenderJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "claw-code-main")
	if err := writeFixtureBridge(root); err != nil {
		t.Fatalf("writeFixtureBridge: %v", err)
	}
	summary, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	payload, err := Lookup(summary, "commands", "agents", 3)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	item, ok := payload["family"].(FamilySummary)
	if !ok || item.Name != "agents" || item.Count != 2 {
		t.Fatalf("unexpected family payload: %#v", payload)
	}

	rendered, err := RenderJSON(summary, "summary", "", 2)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if !strings.Contains(rendered, "\"commands_count\": 5") || !strings.Contains(rendered, "\"top_commands\"") {
		t.Fatalf("unexpected rendered output: %s", rendered)
	}
}

func writeFixtureBridge(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "src", "reference_data", "subsystems"), 0o755); err != nil {
		return err
	}

	commands := []snapshotEntry{
		{Name: "agents", SourceHint: "commands/agents/index.ts"},
		{Name: "agents", SourceHint: "commands/agents/agents.tsx"},
		{Name: "tasks", SourceHint: "commands/tasks/tasks.tsx"},
		{Name: "review", SourceHint: "commands/review/index.ts"},
		{Name: "advisor", SourceHint: "commands/advisor.ts"},
	}
	tools := []snapshotEntry{
		{Name: "AgentTool", SourceHint: "tools/AgentTool/AgentTool.tsx"},
		{Name: "agentMemory", SourceHint: "tools/AgentTool/agentMemory.ts"},
		{Name: "BashTool", SourceHint: "tools/BashTool/BashTool.tsx"},
		{Name: "ReadFileTool", SourceHint: "tools/ReadFileTool/ReadFileTool.tsx"},
	}
	subsystems := []subsystemRecord{
		{ArchiveName: "assistant", ModuleCount: 12, SampleFiles: []string{"assistant/sessionHistory.ts"}},
		{ArchiveName: "cli", ModuleCount: 19, SampleFiles: []string{"cli/handlers/agents.ts"}},
	}

	if err := writeJSON(filepath.Join(root, "src", "reference_data", "commands_snapshot.json"), commands); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, "src", "reference_data", "tools_snapshot.json"), tools); err != nil {
		return err
	}
	for _, item := range subsystems {
		name := item.ArchiveName
		if name == "" {
			name = item.PackageName
		}
		if err := writeJSON(filepath.Join(root, "src", "reference_data", "subsystems", name+".json"), item); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
