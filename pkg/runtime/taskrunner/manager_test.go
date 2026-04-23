package taskrunner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type plannerStub struct {
	response *llm.Response
	err      error
}

func (p plannerStub) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.response, nil
}

func (p plannerStub) Name() string {
	return "planner-stub"
}

func TestCreateBuildsPlanAndSteps(t *testing.T) {
	manager, store, _ := newTaskManagerTest(t, nil)

	task, err := manager.Create(CreateOptions{
		Input:     "inspect the repo and fix the issue",
		Assistant: "main-agent",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.Title != state.ShortenTitle("inspect the repo and fix the issue") {
		t.Fatalf("expected title to default from input, got %q", task.Title)
	}
	if task.Status != "queued" {
		t.Fatalf("expected queued task, got %q", task.Status)
	}
	if task.RecoveryPoint == nil || task.RecoveryPoint.Kind != "queued" {
		t.Fatalf("expected queued recovery point, got %#v", task.RecoveryPoint)
	}
	if len(task.Evidence) == 0 || task.Evidence[0].Kind != "plan" {
		t.Fatalf("expected plan evidence, got %#v", task.Evidence)
	}

	fresh, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatalf("expected task %s to be stored", task.ID)
	}
	steps := store.ListTaskSteps(task.ID)
	if len(steps) != 5 {
		t.Fatalf("expected 5 default plan steps, got %d", len(steps))
	}
	if steps[0].Kind != "analyze" || steps[0].Input != task.Input {
		t.Fatalf("unexpected first step: %+v", steps[0])
	}
	if steps[1].Kind != "inspect" || steps[2].Kind != "execute" || steps[3].Kind != "verify" || steps[4].Kind != "summarize" {
		t.Fatalf("unexpected step kinds: %+v", steps)
	}
	if fresh.PlanSummary == "" {
		t.Fatal("expected plan summary to be recorded")
	}
}

func TestPlanTaskUsesPlannerJSONAndEnsuresRequiredSteps(t *testing.T) {
	manager, _, _ := newTaskManagerTest(t, plannerStub{
		response: &llm.Response{
			Content: "```json\n{\"summary\":\"Do the work\",\"steps\":[{\"title\":\"Analyze input\",\"kind\":\"analyze\"},{\"title\":\"Execute patch\",\"kind\":\"execute\"}]}\n```",
		},
	})

	summary, steps := manager.planTask(context.Background(), "patch it")
	if summary != "Do the work" {
		t.Fatalf("expected planner summary, got %q", summary)
	}
	if len(steps) != 4 {
		t.Fatalf("expected verify and summarize to be appended, got %d steps", len(steps))
	}
	if steps[0].Kind != "analyze" || steps[1].Kind != "execute" || steps[2].Kind != "verify" || steps[3].Kind != "summarize" {
		t.Fatalf("unexpected planned steps: %+v", steps)
	}
}

func TestMarkRejectedUpdatesTaskAndSteps(t *testing.T) {
	manager, store, _ := newTaskManagerTest(t, nil)
	task, err := manager.Create(CreateOptions{
		Title: "Reject me",
		Input: "do not run this",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := manager.MarkRejected(task.ID, 3, "approval denied"); err != nil {
		t.Fatalf("MarkRejected: %v", err)
	}

	fresh, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatalf("expected task %s to exist", task.ID)
	}
	if fresh.Status != "failed" || fresh.Error != "approval denied" {
		t.Fatalf("unexpected rejected task state: %+v", fresh)
	}
	if fresh.RecoveryPoint == nil || fresh.RecoveryPoint.Kind != "failed" {
		t.Fatalf("expected failed recovery point, got %#v", fresh.RecoveryPoint)
	}

	steps := store.ListTaskSteps(task.ID)
	if steps[0].Status != "completed" {
		t.Fatalf("expected analyze step to be completed, got %q", steps[0].Status)
	}
	if steps[1].Status != "skipped" || steps[2].Status != "failed" || steps[3].Status != "skipped" || steps[4].Status != "skipped" {
		t.Fatalf("unexpected step statuses after rejection: %+v", steps)
	}
}

func TestExecutionModeIncludesApprovalAndStrictInspect(t *testing.T) {
	manager, store, _ := newTaskManagerTest(t, nil)
	task, err := manager.Create(CreateOptions{
		Input: "inspect and repair",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	task.PlanSummary = "Inspect the workspace carefully"

	if err := store.AppendApproval(&state.Approval{
		ID:          "approval-1",
		TaskID:      task.ID,
		ToolName:    "task_execution",
		Action:      "execute_task",
		Status:      "approved",
		RequestedAt: manager.nowFunc(),
	}); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}

	mode := manager.executionMode(task)
	if mode.PendingApprovalID != "approval-1" {
		t.Fatalf("expected pending approval id approval-1, got %q", mode.PendingApprovalID)
	}
	if !mode.StrictSteps {
		t.Fatal("expected inspect wording to enable strict steps")
	}
}

func TestAwaitApprovalsIfNeededRequestsAndWaits(t *testing.T) {
	manager, store, sessions := newTaskManagerTest(t, nil)
	task, err := manager.Create(CreateOptions{Input: "dangerous task"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	session, err := sessions.Create("Task session", "main-agent", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	err = manager.awaitApprovalsIfNeeded(task, session, dangerousConfig(), 0)
	if err != ErrTaskWaitingApproval {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}

	approvals := store.ListTaskApprovals(task.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected one approval request, got %d", len(approvals))
	}
	if approvals[0].ToolName != "task_execution" || approvals[0].Action != "execute_task" {
		t.Fatalf("unexpected approval request: %#v", approvals[0])
	}

	freshTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatalf("expected task %s to exist", task.ID)
	}
	if freshTask.Status != "waiting_approval" {
		t.Fatalf("expected waiting_approval task status, got %q", freshTask.Status)
	}
	if freshTask.RecoveryPoint == nil || freshTask.RecoveryPoint.Kind != "approval" {
		t.Fatalf("expected approval recovery point, got %#v", freshTask.RecoveryPoint)
	}
	steps := store.ListTaskSteps(task.ID)
	if steps[1].Status != "waiting_approval" {
		t.Fatalf("expected prepare step to wait for approval, got %q", steps[1].Status)
	}
	updatedSession, ok := sessions.Get(session.ID)
	if !ok {
		t.Fatalf("expected session %s to exist", session.ID)
	}
	if updatedSession.Presence != "waiting_approval" || updatedSession.Typing {
		t.Fatalf("unexpected session state while waiting approval: %+v", updatedSession)
	}
}

func TestAwaitApprovalsIfNeededResumesApprovedExecution(t *testing.T) {
	manager, store, sessions := newTaskManagerTest(t, nil)
	task, err := manager.Create(CreateOptions{Input: "dangerous task"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	session, err := sessions.Create("Task session", "main-agent", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if err := store.AppendApproval(&state.Approval{
		ID:          "approval-2",
		TaskID:      task.ID,
		SessionID:   session.ID,
		StepIndex:   2,
		ToolName:    "task_execution",
		Action:      "execute_task",
		Status:      "approved",
		RequestedAt: manager.nowFunc(),
	}); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}

	if err := manager.awaitApprovalsIfNeeded(task, session, dangerousConfig(), 0); err != nil {
		t.Fatalf("awaitApprovalsIfNeeded: %v", err)
	}

	freshTask, ok := store.GetTask(task.ID)
	if !ok {
		t.Fatalf("expected task %s to exist", task.ID)
	}
	if freshTask.Status != "running" {
		t.Fatalf("expected running task after approved execution, got %q", freshTask.Status)
	}
	steps := store.ListTaskSteps(task.ID)
	if steps[1].Status != "running" {
		t.Fatalf("expected prepare step to resume running, got %q", steps[1].Status)
	}
	updatedSession, ok := sessions.Get(session.ID)
	if !ok {
		t.Fatalf("expected session %s to exist", session.ID)
	}
	if updatedSession.Presence != "typing" || !updatedSession.Typing {
		t.Fatalf("unexpected session state after approval: %+v", updatedSession)
	}
}

func TestRequireToolApprovalHandlesDangerousAndSafeTools(t *testing.T) {
	t.Run("requests approval for dangerous tool", func(t *testing.T) {
		manager, store, sessions := newTaskManagerTest(t, nil)
		task, err := manager.Create(CreateOptions{Input: "run a command"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		session, err := sessions.Create("Task session", "main-agent", "org-1", "project-1", "workspace-1")
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}

		args := map[string]any{"command": "rm -rf /tmp/demo"}
		err = manager.requireToolApproval(task, session, dangerousConfig(), "run_command", args)
		if err != ErrTaskWaitingApproval {
			t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
		}

		approvals := store.ListTaskApprovals(task.ID)
		if len(approvals) != 1 {
			t.Fatalf("expected one tool approval, got %d", len(approvals))
		}
		if approvals[0].ToolName != "run_command" || approvals[0].Action != "tool_call" {
			t.Fatalf("unexpected tool approval: %#v", approvals[0])
		}
		steps := store.ListTaskSteps(task.ID)
		if steps[2].Status != "waiting_approval" {
			t.Fatalf("expected execute step to wait for approval, got %q", steps[2].Status)
		}
	})

	t.Run("allows approved dangerous tool and bypasses safe tool", func(t *testing.T) {
		manager, store, sessions := newTaskManagerTest(t, nil)
		task, err := manager.Create(CreateOptions{Input: "safe and approved"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		session, err := sessions.Create("Task session", "main-agent", "org-1", "project-1", "workspace-1")
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}

		args := map[string]any{"command": "echo hi"}
		if err := store.AppendApproval(&state.Approval{
			ID:          "approval-3",
			TaskID:      task.ID,
			SessionID:   session.ID,
			StepIndex:   3,
			ToolName:    "run_command",
			Action:      "tool_call",
			Signature:   approvalSignature("run_command", "tool_call", args),
			Status:      "approved",
			RequestedAt: manager.nowFunc(),
		}); err != nil {
			t.Fatalf("AppendApproval: %v", err)
		}

		if err := manager.requireToolApproval(task, session, dangerousConfig(), "run_command", args); err != nil {
			t.Fatalf("expected approved dangerous tool to proceed, got %v", err)
		}
		if err := manager.requireToolApproval(task, session, dangerousConfig(), "read_file", map[string]any{"path": "README.md"}); err != nil {
			t.Fatalf("expected safe tool to bypass approval, got %v", err)
		}
	})
}

func TestExecutionAndVerificationHelpers(t *testing.T) {
	task := &state.Task{}
	if got := executionStageOutput(task, nil); got != "Execution completed using the current runtime." {
		t.Fatalf("unexpected default execution output: %q", got)
	}

	output := executionStageOutput(task, newToolActivities(
		agentToolActivityLike{ToolName: "run_command"},
		agentToolActivityLike{ToolName: "write_file"},
	))
	if !strings.Contains(output, "run_command") || !strings.Contains(output, "write_file") {
		t.Fatalf("expected tool names in execution output, got %q", output)
	}

	verifyOutput, observed := verificationStageOutput(task, newToolActivities(
		agentToolActivityLike{ToolName: "desktop_verify_text"},
		agentToolActivityLike{ToolName: "write_file"},
	))
	if !observed || !strings.Contains(verifyOutput, "desktop_verify_text") {
		t.Fatalf("expected observed verification output, got observed=%v output=%q", observed, verifyOutput)
	}
}

type agentToolActivityLike struct {
	ToolName string
}

func newToolActivities(items ...agentToolActivityLike) []agent.ToolActivity {
	results := make([]agent.ToolActivity, len(items))
	for i, item := range items {
		results[i] = agent.ToolActivity{ToolName: item.ToolName}
	}
	return results
}

func newTaskManagerTest(t *testing.T, planner Planner) (*Manager, *state.Store, *state.SessionManager) {
	t.Helper()

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessions := state.NewSessionManager(store, nil)
	approvalManager := state.NewApprovalManager(store)
	manager := NewManager(store, sessions, nil, MainRuntimeInfo{Name: "main-agent"}, planner, approvalManager)

	fixedNow := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	counter := 0
	manager.nowFunc = func() time.Time { return fixedNow }
	manager.nextID = func(prefix string) string {
		counter++
		return fmt.Sprintf("%s-%d", prefix, counter)
	}
	return manager, store, sessions
}

func dangerousConfig() *config.Config {
	return &config.Config{
		Agent: config.AgentConfig{
			RequireConfirmationForDangerous: true,
		},
	}
}
