package gateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	appstate "github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type stubTaskLLM struct {
	responses []*llm.Response
	index     int
	messages  [][]llm.Message
}

func (s *stubTaskLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
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

func TestTaskExecuteInjectsWorkflowSuggestionsIntoAgentHistory(t *testing.T) {
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
	agentLLM := &stubTaskLLM{responses: []*llm.Response{{Content: "done"}}}
	ag := agent.New(agent.Config{
		Name:        "assistant",
		Description: "test assistant",
		LLM:         agentLLM,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		WorkDir:     t.TempDir(),
	})
	app := &appRuntime.App{
		Config: &config.Config{
			Agent: config.AgentConfig{
				Name: "assistant",
			},
		},
		Agent:      ag,
		WorkingDir: workspacePath,
		WorkDir:    t.TempDir(),
	}
	app.Plugins = newWorkflowRegistryForTest(t)

	sessions := NewSessionManager(store, ag)
	pool := NewRuntimePool("ignored", store, 4, time.Hour)
	pool.runtimes[runtimeKey("assistant", "org-1", "project-1", "workspace-1")] = &runtimeEntry{
		app:        app,
		createdAt:  time.Now().UTC(),
		lastUsedAt: time.Now().UTC(),
	}
	manager := NewTaskManager(store, sessions, pool, taskAppInfo{Name: "assistant", WorkingDir: workspacePath}, nil, nil, nil, nil)

	task, err := manager.Create(TaskCreateOptions{
		Input:     "remove the background from this image and export png",
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
		t.Fatal("expected execution result")
	}
	if len(agentLLM.messages) == 0 {
		t.Fatal("expected LLM messages to be captured")
	}
	foundSuggestion := false
	for _, msg := range agentLLM.messages[0] {
		if msg.Role == "system" && strings.Contains(msg.Content, "Suggested app workflows") && strings.Contains(msg.Content, "app_image_app_workflow_remove_background") {
			foundSuggestion = true
			break
		}
	}
	if !foundSuggestion {
		t.Fatalf("expected workflow guidance in LLM messages, got %#v", agentLLM.messages[0])
	}
	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if !strings.Contains(updatedTask.PlanSummary, "Suggested workflows: app_image_app_workflow_remove_background") {
		t.Fatalf("expected plan summary to include suggested workflow, got %q", updatedTask.PlanSummary)
	}
	if !containsEvidenceKind(updatedTask, "workflow_suggestions") {
		t.Fatalf("expected workflow suggestion evidence, got %#v", evidenceKinds(updatedTask))
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
	app := &appRuntime.App{
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
	pool.runtimes[runtimeKey("assistant", "org-1", "project-1", "workspace-1")] = &runtimeEntry{
		app:        app,
		createdAt:  time.Now().UTC(),
		lastUsedAt: time.Now().UTC(),
	}
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

	hook := manager.desktopPlanStateHook(task)
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

func TestTaskExecuteSolidifiesWorkflowDesktopPlanVerifySummaryPath(t *testing.T) {
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
	toolRegistry := tools.NewRegistry()
	toolRegistry.RegisterTool("desktop_open", "open desktop app", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "opened", nil
	})
	toolRegistry.RegisterTool("desktop_verify_text", "verify text", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return `{"matched":true}`, nil
	})

	pluginRegistry, pluginDir := newWorkflowRegistryWithDesktopPlanForTest(t)
	pluginRegistry.RegisterAppPlugins(toolRegistry, pluginDir, filepath.Join(pluginDir, "anyclaw.json"))

	workflowToolName := plugin.AppWorkflowToolName("image-app", "remove-background")
	agentLLM := &stubTaskLLM{responses: []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      workflowToolName,
						Arguments: `{"task":"remove background and export png"}`,
					},
				},
			},
		},
		{Content: "background removed and exported"},
	}}
	ag := agent.New(agent.Config{
		Name:        "assistant",
		Description: "test assistant",
		LLM:         agentLLM,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       toolRegistry,
		WorkDir:     t.TempDir(),
		WorkingDir:  workspacePath,
	})
	app := &appRuntime.App{
		Config: &config.Config{
			Agent: config.AgentConfig{
				Name: "assistant",
			},
		},
		Agent:      ag,
		WorkingDir: workspacePath,
		WorkDir:    t.TempDir(),
		Plugins:    pluginRegistry,
		Tools:      toolRegistry,
	}

	sessions := NewSessionManager(store, ag)
	pool := NewRuntimePool("ignored", store, 4, time.Hour)
	pool.runtimes[runtimeKey("assistant", "org-1", "project-1", "workspace-1")] = &runtimeEntry{
		app:        app,
		createdAt:  time.Now().UTC(),
		lastUsedAt: time.Now().UTC(),
	}
	manager := NewTaskManager(store, sessions, pool, taskAppInfo{Name: "assistant", WorkingDir: workspacePath}, nil, nil, nil, pluginRegistry)

	task, err := manager.Create(TaskCreateOptions{
		Input:     "remove the background from this image and export png",
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
		t.Fatal("expected execution result")
	}

	steps := manager.Steps(task.ID)
	if len(steps) != 5 {
		t.Fatalf("expected 5 main-path steps, got %d details=%v", len(steps), stepDetails(steps))
	}
	if steps[1].Kind != "workflow" || steps[1].Status != "completed" || steps[1].ToolName != workflowToolName {
		t.Fatalf("expected workflow step to complete, got %#v", steps[1])
	}
	if steps[2].Kind != "desktop_plan" || steps[2].Status != "completed" {
		t.Fatalf("expected desktop plan step to complete, got %#v", steps[2])
	}
	if steps[3].Kind != "verify" || steps[3].Status != "completed" {
		t.Fatalf("expected verify step to complete, got %#v", steps[3])
	}
	if steps[4].Kind != "summarize" || steps[4].Status != "completed" {
		t.Fatalf("expected summarize step to complete, got %#v", steps[4])
	}

	updatedTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatal("expected task to exist")
	}
	if updatedTask.ExecutionState == nil || updatedTask.ExecutionState.DesktopPlan == nil {
		t.Fatalf("expected desktop plan execution state, got %#v", updatedTask.ExecutionState)
	}
	if !desktopPlanHasExplicitVerification(updatedTask.ExecutionState.DesktopPlan) {
		t.Fatalf("expected desktop plan verification to be recorded, got %#v", updatedTask.ExecutionState.DesktopPlan)
	}
	if !containsEvidenceKind(updatedTask, "workflow_selected") {
		t.Fatalf("expected workflow_selected evidence, got %#v", evidenceKinds(updatedTask))
	}
	if !containsEvidenceKind(updatedTask, "verification_completed") {
		t.Fatalf("expected verification_completed evidence, got %#v", evidenceKinds(updatedTask))
	}
}

func newWorkflowRegistryForTest(t *testing.T) *plugin.Registry {
	t.Helper()
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "image-app")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := plugin.Manifest{
		Name:        "image-app",
		Version:     "1.0.0",
		Enabled:     true,
		Entrypoint:  "app.py",
		Permissions: []string{"tool:exec"},
		App: &plugin.AppSpec{
			Name: "Image App",
			Actions: []plugin.AppActionSpec{
				{Name: "run"},
			},
			Workflows: []plugin.AppWorkflowSpec{
				{
					Name:        "remove-background",
					Description: "Remove the background and export png",
					Action:      "run",
					Tags:        []string{"background", "png", "image"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile entrypoint: %v", err)
	}
	registry, err := plugin.NewRegistry(config.PluginsConfig{
		Dir:          baseDir,
		AllowExec:    true,
		RequireTrust: false,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func newWorkflowRegistryWithDesktopPlanForTest(t *testing.T) (*plugin.Registry, string) {
	t.Helper()
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "image-app")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := plugin.Manifest{
		Name:        "image-app",
		Version:     "1.0.0",
		Enabled:     true,
		Entrypoint:  "app.ps1",
		Permissions: []string{"tool:exec"},
		App: &plugin.AppSpec{
			Name: "Image App",
			Actions: []plugin.AppActionSpec{
				{Name: "run"},
			},
			Workflows: []plugin.AppWorkflowSpec{
				{
					Name:        "remove-background",
					Description: "Remove the background and export png",
					Action:      "run",
					Tags:        []string{"background", "png", "image"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	script := `$payload = @{
  protocol = "anyclaw.app.desktop.v1"
  summary = "background removed"
  steps = @(
    @{
      tool = "desktop_open"
      label = "Launch image app"
      input = @{ target = "image-app.exe" }
      verify = @{
        tool = "desktop_verify_text"
        input = @{ expected = "ready" }
        retry = 1
      }
    }
  )
} | ConvertTo-Json -Depth 8 -Compress
Write-Output $payload
`
	if err := os.WriteFile(filepath.Join(pluginDir, "app.ps1"), []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile entrypoint: %v", err)
	}
	registry, err := plugin.NewRegistry(config.PluginsConfig{
		Dir:          baseDir,
		AllowExec:    true,
		RequireTrust: false,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry, baseDir
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
