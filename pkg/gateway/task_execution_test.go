package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	appstate "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

type (
	Org               = state.Org
	Project           = state.Project
	Workspace         = state.Workspace
	Approval          = state.Approval
	Task              = state.Task
	TaskStep          = state.TaskStep
	taskAppInfo       = taskrunner.MainRuntimeInfo
	TaskCreateOptions = taskrunner.CreateOptions
)

var ErrTaskWaitingApproval = taskrunner.ErrTaskWaitingApproval

func NewStore(baseDir string) (*state.Store, error) {
	return state.NewStore(baseDir)
}

func NewSessionManager(store *state.Store, agent state.SessionAgent) *state.SessionManager {
	return state.NewSessionManager(store, agent)
}

func NewRuntimePool(configPath string, store *state.Store, maxInstances int, idleTTL time.Duration) *appRuntime.RuntimePool {
	return appRuntime.NewRuntimePool(configPath, store, maxInstances, idleTTL)
}

func newApprovalManager(store *state.Store) *state.ApprovalManager {
	return state.NewApprovalManager(store)
}

func NewTaskManager(store *state.Store, sessions *state.SessionManager, pool *appRuntime.RuntimePool, app taskAppInfo, planner taskrunner.Planner, approvals *state.ApprovalManager, _ any, _ any) *taskrunner.Manager {
	return taskrunner.NewManager(store, sessions, pool, app, planner, approvals)
}

func desktopPlanHasExplicitVerification(state *appstate.DesktopPlanExecutionState) bool {
	return taskrunner.DesktopPlanHasExplicitVerification(state)
}

type stubTaskLLM struct {
	responses []*llm.Response
	index     int
	messages  [][]llm.Message
	chatFunc  func(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error)
}

func (s *stubTaskLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
	if s.chatFunc != nil {
		return s.chatFunc(ctx, messages, toolDefs)
	}
	if s.index >= len(s.responses) {
		return &llm.Response{Content: "done"}, nil
	}
	resp := s.responses[s.index]
	s.index++
	return resp, nil
}

func (s *stubTaskLLM) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	resp, err := s.Chat(ctx, messages, toolDefs)
	if err != nil {
		return err
	}
	if resp != nil && onChunk != nil {
		onChunk(resp.Content)
	}
	return nil
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
	mainRuntime := &appRuntime.App{
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
	pool.Remember("assistant", "org-1", "project-1", "workspace-1", mainRuntime)
	approvals := newApprovalManager(store)
	manager := NewTaskManager(store, sessions, pool, taskAppInfo{Name: "assistant", WorkingDir: workspacePath}, nil, approvals, nil, nil)

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
	if !errors.Is(err, ErrTaskWaitingApproval) {
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
	if updatedTask.RecoveryPoint == nil || updatedTask.RecoveryPoint.Kind != "approval" {
		t.Fatalf("expected approval recovery point, got %#v", updatedTask.RecoveryPoint)
	}
	if updatedTask.RecoveryPoint.ToolName != "run_command" {
		t.Fatalf("expected recovery point tool run_command, got %#v", updatedTask.RecoveryPoint)
	}
	if !containsEvidenceKind(updatedTask, "approval_waiting") {
		t.Fatalf("expected approval_waiting evidence, got %#v", evidenceKinds(updatedTask))
	}
}

func TestTaskCreateInitializesRecoveryScaffold(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewTaskManager(store, nil, nil, taskAppInfo{}, nil, nil, nil, nil)

	task, err := manager.Create(TaskCreateOptions{Input: "draft the release notes"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if updatedTask.RecoveryPoint == nil || updatedTask.RecoveryPoint.Kind != "queued" {
		t.Fatalf("expected queued recovery point, got %#v", updatedTask.RecoveryPoint)
	}
	if !containsEvidenceKind(updatedTask, "plan") {
		t.Fatalf("expected plan evidence, got %#v", evidenceKinds(updatedTask))
	}
}

func TestTaskMarkRejectedUsesApprovalStepIndex(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewTaskManager(store, nil, nil, taskAppInfo{}, nil, nil, nil, nil)
	task, err := manager.Create(TaskCreateOptions{Input: "review this"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	steps := manager.Steps(task.ID)
	if len(steps) < 3 {
		t.Fatalf("expected planned steps, got %+v", steps)
	}
	if err := manager.SetStepStatus(task.ID, 1, "completed", "review this", "accepted", ""); err != nil {
		t.Fatalf("setStepStatus step1: %v", err)
	}
	if err := manager.SetStepStatus(task.ID, 2, "completed", "", "executed", ""); err != nil {
		t.Fatalf("setStepStatus step2: %v", err)
	}
	if err := manager.SetStepStatus(task.ID, 3, "waiting_approval", "", "pending tool approval", ""); err != nil {
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
	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if updatedTask.RecoveryPoint == nil || updatedTask.RecoveryPoint.Kind != "failed" {
		t.Fatalf("expected failed recovery point, got %#v", updatedTask.RecoveryPoint)
	}
	if !containsEvidenceKind(updatedTask, "approval_rejected") {
		t.Fatalf("expected approval_rejected evidence, got %#v", evidenceKinds(updatedTask))
	}
}

func TestTaskExecutePersistsEvidenceArtifactsAndCompletionRecoveryPoint(t *testing.T) {
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
	registry.RegisterTool("write_file", "write", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "wrote report.txt", nil
	})
	agentLLM := &stubTaskLLM{responses: []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"report.txt","content":"hello"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	ag := agent.New(agent.Config{
		Name:        "assistant",
		Description: "test assistant",
		LLM:         agentLLM,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		WorkDir:     t.TempDir(),
	})
	mainRuntime := &appRuntime.App{
		Config: &config.Config{
			Agent: config.AgentConfig{
				Name: "assistant",
			},
		},
		Agent:      ag,
		WorkingDir: workspacePath,
		WorkDir:    t.TempDir(),
	}
	sessions := NewSessionManager(store, ag)
	pool := NewRuntimePool("ignored", store, 4, time.Hour)
	pool.Remember("assistant", "org-1", "project-1", "workspace-1", mainRuntime)
	manager := NewTaskManager(store, sessions, pool, taskAppInfo{Name: "assistant", WorkingDir: workspacePath}, nil, nil, nil, nil)

	task, err := manager.Create(TaskCreateOptions{
		Input:     "write a report file",
		Assistant: "assistant",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := manager.Execute(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || result.Task == nil {
		t.Fatal("expected task execution result")
	}
	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if updatedTask.Status != "completed" {
		t.Fatalf("expected completed task, got %#v", updatedTask)
	}
	if updatedTask.RecoveryPoint == nil || updatedTask.RecoveryPoint.Kind != "completed" {
		t.Fatalf("expected completed recovery point, got %#v", updatedTask.RecoveryPoint)
	}
	if !containsEvidenceKind(updatedTask, "execution_started") || !containsEvidenceKind(updatedTask, "tool_activity") || !containsEvidenceKind(updatedTask, "task_completed") {
		t.Fatalf("expected execution/tool/completion evidence, got %#v", evidenceKinds(updatedTask))
	}
	if len(updatedTask.Artifacts) == 0 {
		t.Fatalf("expected task artifacts, got %#v", updatedTask)
	}
	foundArtifact := false
	for _, artifact := range updatedTask.Artifacts {
		if artifact != nil && artifact.ToolName == "write_file" && artifact.Path == "report.txt" {
			foundArtifact = true
			break
		}
	}
	if !foundArtifact {
		t.Fatalf("expected write_file artifact, got %#v", updatedTask.Artifacts)
	}
}

func TestDesktopPlanStateHookPersistsTaskExecutionState(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewTaskManager(store, nil, nil, taskAppInfo{}, nil, nil, nil, nil)
	task, err := manager.Create(TaskCreateOptions{Input: "resume this desktop workflow"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	hook := manager.DesktopPlanStateHook(task)
	if hook == nil {
		t.Fatal("expected desktop plan state hook")
	}
	hook(context.Background(), appstate.DesktopPlanExecutionState{
		ToolName:          "app_demo_run",
		Status:            "running",
		TotalSteps:        3,
		CurrentStep:       2,
		NextStep:          2,
		LastCompletedStep: 1,
		Steps: []appstate.DesktopPlanStepExecutionState{
			{Index: 1, Tool: "desktop_open", Status: "completed", Output: "Launch: opened"},
			{Index: 2, Tool: "desktop_click", Status: "running"},
		},
	})

	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if updatedTask.ExecutionState == nil || updatedTask.ExecutionState.DesktopPlan == nil {
		t.Fatal("expected desktop plan execution state to be persisted")
	}
	if updatedTask.ExecutionState.DesktopPlan.ToolName != "app_demo_run" {
		t.Fatalf("unexpected tool name: %#v", updatedTask.ExecutionState.DesktopPlan)
	}
	if updatedTask.ExecutionState.DesktopPlan.NextStep != 2 || updatedTask.ExecutionState.DesktopPlan.LastCompletedStep != 1 {
		t.Fatalf("unexpected execution checkpoint: %#v", updatedTask.ExecutionState.DesktopPlan)
	}
	if updatedTask.RecoveryPoint == nil || updatedTask.RecoveryPoint.Kind != "desktop_plan" {
		t.Fatalf("expected desktop_plan recovery point, got %#v", updatedTask.RecoveryPoint)
	}
	if !containsEvidenceKind(updatedTask, "desktop_checkpoint") {
		t.Fatalf("expected desktop_checkpoint evidence, got %#v", evidenceKinds(updatedTask))
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

func evidenceKinds(task *Task) []string {
	result := make([]string, 0, len(task.Evidence))
	for _, evidence := range task.Evidence {
		if evidence == nil {
			continue
		}
		result = append(result, evidence.Kind)
	}
	return result
}

func containsEvidenceKind(task *Task, kind string) bool {
	for _, item := range evidenceKinds(task) {
		if item == kind {
			return true
		}
	}
	return false
}
