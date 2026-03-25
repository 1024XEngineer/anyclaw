package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type stubTaskLLM struct {
	responses []*llm.Response
	index     int
}

func (s *stubTaskLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	if s.index >= len(s.responses) {
		return &llm.Response{Content: "done"}, nil
	}
	resp := s.responses[s.index]
	s.index++
	return resp, nil
}

func (s *stubTaskLLM) Name() string {
	return "stub"
}

func TestTaskExecuteWaitsForToolApprovalWithoutFailingTask(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	workspacePath := t.TempDir()
	if err := store.UpsertWorkspace(&Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: workspacePath}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "run", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "ok", nil
	})
	ag := agent.New(agent.Config{
		Name:        "assistant",
		Description: "test assistant",
		LLM: &stubTaskLLM{responses: []*llm.Response{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tool-1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "run_command",
							Arguments: `{"command":"echo hi"}`,
						},
					},
				},
			},
		}},
		Memory:  mem,
		Skills:  skills.NewSkillsManager(""),
		Tools:   registry,
		WorkDir: t.TempDir(),
	})
	app := &appRuntime.App{
		Config: &config.Config{
			Agent: config.AgentConfig{
				Name:                            "assistant",
				RequireConfirmationForDangerous: true,
			},
		},
		Agent:      ag,
		WorkingDir: workspacePath,
		WorkDir:    t.TempDir(),
	}
	sessions := NewSessionManager(store, ag)
	pool := NewRuntimePool("ignored", store, 4, time.Hour)
	pool.runtimes[runtimeKey("assistant", "org-1", "project-1", "workspace-1")] = &runtimeEntry{
		app:        app,
		createdAt:  time.Now().UTC(),
		lastUsedAt: time.Now().UTC(),
	}
	approvals := newApprovalManager(store)
	manager := NewTaskManager(store, sessions, pool, taskAppInfo{Name: "assistant", WorkingDir: workspacePath}, nil, approvals)

	task, err := manager.Create(TaskCreateOptions{
		Input:     "run a command",
		Assistant: "assistant",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.AppendApproval(&Approval{
		ID:          "approval-exec",
		TaskID:      task.ID,
		StepIndex:   2,
		ToolName:    "task_execution",
		Action:      "execute_task",
		Signature:   "approved",
		Status:      "approved",
		RequestedAt: time.Now().UTC(),
		ResolvedAt:  time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}

	result, err := manager.Execute(context.Background(), task.ID)
	if err == nil || err != ErrTaskWaitingApproval {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}
	if result == nil || result.Task == nil || result.Session == nil {
		t.Fatal("expected task execution result with task and session")
	}
	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if updatedTask.Status != "waiting_approval" {
		t.Fatalf("expected task status waiting_approval, got %q", updatedTask.Status)
	}
	session, ok := sessions.Get(result.Session.ID)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if session.Presence != "waiting_approval" || session.Typing {
		t.Fatalf("expected session waiting_approval without typing, got presence=%q typing=%v", session.Presence, session.Typing)
	}
	approvalsList := store.ListTaskApprovals(task.ID)
	if len(approvalsList) != 2 {
		t.Fatalf("expected 2 approvals, got %d", len(approvalsList))
	}
	foundToolApproval := false
	for _, approval := range approvalsList {
		if approval.ToolName == "run_command" && approval.Action == "tool_call" && approval.Status == "pending" {
			foundToolApproval = true
		}
	}
	if !foundToolApproval {
		payloads := make([]string, 0, len(approvalsList))
		for _, approval := range approvalsList {
			raw, _ := json.Marshal(approval)
			payloads = append(payloads, string(raw))
		}
		t.Fatalf("expected pending run_command approval, got %v", payloads)
	}
	steps := manager.Steps(task.ID)
	statuses := stepStatusesByIndex(steps)
	if statuses[3] != "waiting_approval" {
		t.Fatalf("expected step 3 to be waiting_approval, got %v details=%v", statuses, stepDetails(steps))
	}
}

func TestTaskMarkRejectedUsesApprovalStepIndex(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewTaskManager(store, nil, nil, taskAppInfo{}, nil, nil)
	task, err := manager.Create(TaskCreateOptions{Input: "review this"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	steps := manager.Steps(task.ID)
	if len(steps) < 3 {
		t.Fatalf("expected planned steps, got %+v", steps)
	}
	if err := manager.setStepStatus(task.ID, 1, "completed", "review this", "accepted", ""); err != nil {
		t.Fatalf("setStepStatus step1: %v", err)
	}
	if err := manager.setStepStatus(task.ID, 2, "completed", "", "executed", ""); err != nil {
		t.Fatalf("setStepStatus step2: %v", err)
	}
	if err := manager.setStepStatus(task.ID, 3, "waiting_approval", "", "pending tool approval", ""); err != nil {
		t.Fatalf("setStepStatus step3: %v", err)
	}
	initialStatuses := stepStatusesByIndex(manager.Steps(task.ID))
	if initialStatuses[1] != "completed" || initialStatuses[2] != "completed" || initialStatuses[3] != "waiting_approval" {
		t.Fatalf("unexpected initial step statuses: %v details=%v", initialStatuses, stepDetails(manager.Steps(task.ID)))
	}

	if err := manager.MarkRejected(task.ID, 3, "denied"); err != nil {
		t.Fatalf("MarkRejected: %v", err)
	}

	updatedStatuses := stepStatusesByIndex(manager.Steps(task.ID))
	if updatedStatuses[1] != "completed" {
		t.Fatalf("expected step 1 to remain completed, got %q", updatedStatuses[1])
	}
	if updatedStatuses[2] != "completed" {
		t.Fatalf("expected step 2 to remain completed, got %q", updatedStatuses[2])
	}
	if updatedStatuses[3] != "failed" {
		t.Fatalf("expected step 3 to fail, got %q", updatedStatuses[3])
	}
}

func stepStatusesByIndex(steps []*TaskStep) map[int]string {
	result := make(map[int]string, len(steps))
	for _, step := range steps {
		result[step.Index] = step.Status
	}
	return result
}

func stepDetails(steps []*TaskStep) []string {
	result := make([]string, 0, len(steps))
	for _, step := range steps {
		result = append(result, step.ID+":"+step.TaskID+":"+step.Title+":"+step.Status)
	}
	return result
}
