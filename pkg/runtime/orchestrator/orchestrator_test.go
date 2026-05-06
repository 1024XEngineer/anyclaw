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

func TestSetToolOptionsRefreshesExistingSubAgentRegistry(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	t.Cleanup(func() { mem.Close() })

	workingDir := t.TempDir()
	fullOpts := tools.BuiltinOptions{WorkingDir: workingDir, PermissionLevel: "full"}
	fullRegistry := tools.NewRegistry()
	tools.RegisterBuiltins(fullRegistry, fullOpts)

	orch, err := NewOrchestrator(OrchestratorConfig{
		MaxConcurrentAgents: 1,
		EnableDecomposition: false,
		ToolOptions:         &fullOpts,
		AgentDefinitions: []AgentDefinition{
			{
				Name:            "worker",
				Description:     "Executes delegated work",
				PermissionLevel: "full",
				WorkingDir:      workingDir,
			},
		},
	}, &orchestratorTestLLM{}, skills.NewSkillsManager(""), fullRegistry, mem)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	sa, ok := orch.GetAgent("worker")
	if !ok {
		t.Fatal("expected worker subagent")
	}
	t.Cleanup(func() {
		if sa.memory != nil && sa.memory != mem {
			_ = sa.memory.Close()
		}
	})
	if !containsToolName(sa.Tools(), "write_file") {
		t.Fatalf("expected full subagent to expose write_file, got %#v", sa.Tools())
	}
	if got := sa.PermissionLevel(); got != "full" {
		t.Fatalf("expected initial permission full, got %q", got)
	}

	readOnlyOpts := tools.BuiltinOptions{WorkingDir: workingDir, PermissionLevel: "read-only"}
	readOnlyRegistry := tools.NewRegistry()
	tools.RegisterBuiltins(readOnlyRegistry, readOnlyOpts)
	orch.SetToolOptions(readOnlyOpts, readOnlyRegistry)

	if containsToolName(sa.Tools(), "write_file") {
		t.Fatalf("expected refreshed read-only subagent to hide write_file, got %#v", sa.Tools())
	}
	if containsToolName(sa.Tools(), "run_command") {
		t.Fatalf("expected refreshed read-only subagent to hide run_command, got %#v", sa.Tools())
	}
	if !containsToolName(sa.Tools(), "read_file") {
		t.Fatalf("expected refreshed read-only subagent to keep read_file, got %#v", sa.Tools())
	}
	if got := sa.PermissionLevel(); got != "read-only" {
		t.Fatalf("expected refreshed permission read-only, got %q", got)
	}
	if infos := sa.agent.ListTools(); containsToolInfo(infos, "write_file") {
		t.Fatalf("expected underlying agent tools to refresh, got %#v", infos)
	}
}

func containsToolName(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsToolInfo(items []tools.ToolInfo, want string) bool {
	for _, item := range items {
		if item.Name == want {
			return true
		}
	}
	return false
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
