package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/capability/models"
	"github.com/anyclaw/anyclaw/pkg/capability/skills"
	"github.com/anyclaw/anyclaw/pkg/capability/tools"
	"github.com/anyclaw/anyclaw/pkg/runtime/orchestrator"
	"github.com/anyclaw/anyclaw/pkg/state/memory"
)

func TestRegisterDelegationToolRegistersMainAgentOnlyTool(t *testing.T) {
	app := &App{
		Tools:        tools.NewRegistry(),
		Orchestrator: newTestOrchestrator(t),
	}

	registerDelegationTool(app)

	mainTools := app.Tools.ListForRole(false)
	subTools := app.Tools.ListForRole(true)
	if !containsTool(mainTools, "delegate_task") {
		t.Fatalf("expected main agent tool list to include delegate_task, got %#v", mainTools)
	}
	if containsTool(subTools, "delegate_task") {
		t.Fatalf("expected sub-agent tool list to hide delegate_task, got %#v", subTools)
	}
}

func TestDelegateTaskToolReturnsStructuredResult(t *testing.T) {
	app := &App{
		Tools:        tools.NewRegistry(),
		Orchestrator: newTestOrchestrator(t),
	}

	registerDelegationTool(app)

	ctx := tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent})
	raw, err := app.Tools.Call(ctx, "delegate_task", map[string]any{
		"task":             "Inspect the repository and summarize the next implementation step.",
		"reason":           "The task benefits from a specialist sub-agent.",
		"success_criteria": "Return a concise actionable summary.",
		"user_context":     "The main agent will integrate the delegated result into the final answer.",
	})
	if err != nil {
		t.Fatalf("delegate_task: %v", err)
	}

	var result DelegationResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal delegation result: %v\nraw=%s", err, raw)
	}
	if result.TaskID == "" {
		t.Fatalf("expected task id, got %#v", result)
	}
	if len(result.SubTasks) == 0 {
		t.Fatalf("expected sub-task details, got %#v", result)
	}
	if !strings.Contains(result.DelegationBrief, "Delegated task:") {
		t.Fatalf("expected delegation brief in result, got %q", result.DelegationBrief)
	}
	if result.Status == "" {
		t.Fatalf("expected status, got %#v", result)
	}
}

func newTestOrchestrator(t *testing.T) *orchestrator.Orchestrator {
	t.Helper()

	mem := newTestMemory(t)
	t.Cleanup(func() { mem.Close() })

	orch, err := orchestrator.NewOrchestrator(orchestrator.OrchestratorConfig{
		MaxConcurrentAgents: 1,
		EnableDecomposition: false,
		AgentDefinitions: []orchestrator.AgentDefinition{
			{
				Name:            "specialist",
				Description:     "Handles delegated sub-tasks",
				PermissionLevel: "limited",
			},
		},
	}, &testRuntimeLLM{}, skills.NewSkillsManager(""), tools.NewRegistry(), mem)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	return orch
}

func containsTool(items []tools.ToolInfo, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func newTestMemory(t *testing.T) memory.MemoryBackend {
	t.Helper()
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	return mem
}

type testRuntimeLLM struct{}

func (t *testRuntimeLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "delegated-result"}, nil
}

func (t *testRuntimeLLM) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	if onChunk != nil {
		onChunk("delegated-result")
	}
	return nil
}

func (t *testRuntimeLLM) Name() string {
	return "runtime-test"
}
