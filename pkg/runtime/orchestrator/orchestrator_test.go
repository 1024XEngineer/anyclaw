package orchestrator

import (
	"context"
	"testing"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

func TestRunTaskResultUsesFreshExecutionStatePerRun(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	t.Cleanup(func() { mem.Close() })

	orch, err := NewOrchestrator(OrchestratorConfig{
		MaxConcurrentAgents: 1,
		EnableDecomposition: false,
		AgentDefinitions: []AgentDefinition{
			{
				Name:            "worker",
				Description:     "Executes delegated work",
				PermissionLevel: "limited",
			},
		},
	}, &orchestratorTestLLM{}, skills.NewSkillsManager(""), tools.NewRegistry(), mem)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	result1, err := orch.RunTaskResult(context.Background(), "first task", []string{"worker"})
	if err != nil {
		t.Fatalf("RunTaskResult(first): %v", err)
	}
	result2, err := orch.RunTaskResult(context.Background(), "second task", []string{"worker"})
	if err != nil {
		t.Fatalf("RunTaskResult(second): %v", err)
	}

	if result1.TaskID == "" || result2.TaskID == "" || result1.TaskID == result2.TaskID {
		t.Fatalf("expected unique task ids, got %q and %q", result1.TaskID, result2.TaskID)
	}
	if len(result1.SubTasks) != 1 || len(result2.SubTasks) != 1 {
		t.Fatalf("expected one sub-task per run, got %d and %d", len(result1.SubTasks), len(result2.SubTasks))
	}
	if result1.SubTasks[0].ID == result2.SubTasks[0].ID {
		t.Fatalf("expected sub-task ids to reset per run, got %q", result1.SubTasks[0].ID)
	}
	if len(result2.History) == 0 {
		t.Fatalf("expected execution history for second run")
	}
	for _, item := range result2.History {
		if item.TaskID != "" && item.TaskID != result2.TaskID && item.TaskID != result2.SubTasks[0].ID {
			t.Fatalf("expected second history to contain only second-run task ids, got %#v", result2.History)
		}
	}
}

func TestRunTemporaryPlanCreatesEphemeralAgentAndCleansItUp(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	t.Cleanup(func() { mem.Close() })

	orch, err := NewOrchestrator(OrchestratorConfig{
		MaxConcurrentAgents: 1,
		EnableDecomposition: false,
		DefaultWorkingDir:   t.TempDir(),
	}, &orchestratorTestLLM{}, skills.NewSkillsManager(""), tools.NewRegistry(), mem)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	result, err := orch.RunTemporaryPlan(context.Background(), "temporary delegated task", "Review Bot")
	if err != nil {
		t.Fatalf("RunTemporaryPlan: %v", err)
	}
	if len(result.SubTasks) != 1 {
		t.Fatalf("expected one temporary sub-task, got %#v", result.SubTasks)
	}
	if result.SubTasks[0].AssignedAgent != "review-bot" {
		t.Fatalf("expected normalized temporary agent name, got %#v", result.SubTasks)
	}
	if orch.AgentCount() != 0 {
		t.Fatalf("expected temporary agent to be cleaned up, got %d agents", orch.AgentCount())
	}
}

type orchestratorTestLLM struct{}

func (o *orchestratorTestLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "worker-output"}, nil
}

func (o *orchestratorTestLLM) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	if onChunk != nil {
		onChunk("worker-output")
	}
	return nil
}

func (o *orchestratorTestLLM) Name() string {
	return "orchestrator-test"
}
