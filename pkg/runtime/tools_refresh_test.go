package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

func TestRefreshToolRegistrySynchronizesMainAgentTools(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.PermissionLevel = "read-only"
	cfg.Agent.WorkingDir = tempDir
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, "audit.jsonl")

	mem := memory.NewFileMemory(tempDir)
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	oldRegistry := tools.NewRegistry()
	oldRegistry.RegisterTool("stale_tool", "stale test tool", nil, func(ctx context.Context, input map[string]any) (string, error) {
		return "stale", nil
	})
	tools.RegisterBuiltins(oldRegistry, tools.BuiltinOptions{
		WorkingDir:      tempDir,
		PermissionLevel: "full",
	})

	ag := agent.New(agent.Config{
		Name:             "main",
		Memory:           mem,
		Skills:           skills.NewSkillsManager(cfg.Skills.Dir),
		Tools:            oldRegistry,
		MaxContextTokens: 4096,
		LLM: &refreshToolLLM{
			toolName: "stale_tool",
		},
	})
	rt := &MainRuntime{
		ConfigPath: filepath.Join(tempDir, "anyclaw.json"),
		Config:     cfg,
		Agent:      ag,
		Memory:     mem,
		Skills:     skills.NewSkillsManager(cfg.Skills.Dir),
		Tools:      oldRegistry,
		WorkDir:    tempDir,
		WorkingDir: tempDir,
	}

	if !hasAgentTool(ag, "stale_tool") {
		t.Fatal("expected stale tool to be visible before refresh")
	}
	result, err := ag.Run(context.Background(), "call stale_tool now")
	if err != nil {
		t.Fatalf("expected old registry to execute stale tool before refresh: %v", err)
	}
	if result != "done" {
		t.Fatalf("expected final LLM response after stale tool call, got %q", result)
	}

	if err := rt.RefreshToolRegistry(); err != nil {
		t.Fatalf("RefreshToolRegistry: %v", err)
	}
	if hasAgentTool(ag, "stale_tool") {
		t.Fatal("expected Agent tool registry to be replaced after refresh")
	}
	if _, ok := rt.Tools.Get("stale_tool"); ok {
		t.Fatal("expected runtime tool registry to be replaced after refresh")
	}

	_, err = ag.Run(context.Background(), "call stale_tool now")
	if err != nil {
		t.Fatalf("expected Agent to continue after stale tool execution error, got %v", err)
	}
	activities := ag.GetLastToolActivities()
	if len(activities) == 0 || activities[0].ToolName != "stale_tool" || !strings.Contains(activities[0].Error, "tool not found: stale_tool") {
		t.Fatalf("expected Agent to execute against refreshed registry and record stale tool error, got %#v", activities)
	}
	_, err = rt.CallTool(context.Background(), "write_file", map[string]any{
		"path":    filepath.Join(tempDir, "after.txt"),
		"content": "after",
	})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected refreshed runtime registry to enforce read-only, got %v", err)
	}
}

type refreshToolLLM struct {
	toolName string
	calls    int
}

func (l *refreshToolLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	l.calls++
	if l.calls%2 == 1 {
		return &llm.Response{
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-1",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      l.toolName,
					Arguments: `{}`,
				},
			}},
		}, nil
	}
	return &llm.Response{Content: "done"}, nil
}

func (l *refreshToolLLM) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error {
	resp, err := l.Chat(ctx, messages, tools)
	if err != nil {
		return err
	}
	if onChunk != nil {
		onChunk(resp.Content)
	}
	return nil
}

func (l *refreshToolLLM) Name() string {
	return "refresh-tool-llm"
}

func hasAgentTool(ag *agent.Agent, name string) bool {
	for _, item := range ag.ListTools() {
		if item.Name == name {
			return true
		}
	}
	return false
}
