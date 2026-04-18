package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/clawbridge"
)

func TestRegisterClawBridgeToolsRegistersContextToolWhenBridgeExists(t *testing.T) {
	bridgeRoot := filepath.Join(t.TempDir(), "claw-code-main")
	if err := writeToolBridgeFixture(bridgeRoot); err != nil {
		t.Fatalf("writeToolBridgeFixture: %v", err)
	}
	t.Setenv(clawbridge.EnvRoot, bridgeRoot)

	registry := NewRegistry()
	RegisterClawBridgeTools(registry, BuiltinOptions{WorkingDir: t.TempDir()})

	tool, ok := registry.Get("claw_bridge_context")
	if !ok {
		t.Fatalf("expected claw_bridge_context to be registered")
	}
	result, err := tool.Handler(context.Background(), map[string]any{"section": "summary", "limit": 2})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	if !strings.Contains(result, "\"commands_count\": 3") || !strings.Contains(result, "\"top_tools\"") {
		t.Fatalf("unexpected tool output: %s", result)
	}
}

func writeToolBridgeFixture(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "src", "reference_data", "subsystems"), 0o755); err != nil {
		return err
	}
	commands := []map[string]string{
		{"name": "agents", "source_hint": "commands/agents/index.ts"},
		{"name": "tasks", "source_hint": "commands/tasks/index.ts"},
		{"name": "tasks", "source_hint": "commands/tasks/tasks.tsx"},
	}
	toolItems := []map[string]string{
		{"name": "AgentTool", "source_hint": "tools/AgentTool/AgentTool.tsx"},
		{"name": "agentMemory", "source_hint": "tools/AgentTool/agentMemory.ts"},
	}
	subsystem := map[string]any{
		"archive_name": "cli",
		"module_count": 19,
		"sample_files": []string{"cli/handlers/agents.ts"},
	}
	if err := writeToolJSON(filepath.Join(root, "src", "reference_data", "commands_snapshot.json"), commands); err != nil {
		return err
	}
	if err := writeToolJSON(filepath.Join(root, "src", "reference_data", "tools_snapshot.json"), toolItems); err != nil {
		return err
	}
	return writeToolJSON(filepath.Join(root, "src", "reference_data", "subsystems", "cli.json"), subsystem)
}

func writeToolJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
