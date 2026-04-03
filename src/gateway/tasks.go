package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/apps"
	appstate "github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/prompt"
	"github.com/anyclaw/anyclaw/pkg/routing"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type TaskManager struct {
	store       *Store
	sessions    *SessionManager
	runtimePool *RuntimePool
	app         taskAppInfo
	planner     taskPlanner
	approvals   *approvalManager
	router      *routing.Router
	registry    *plugin.Registry
	nextID      func(prefix string) string
	nowFunc     func() time.Time
}

type taskAppInfo struct {
	Name       string
	WorkingDir string
	ConfigPath string
}

type taskPlanner interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
	Name() string
}

type TaskCreateOptions struct {
	Title     string
	Input     string
	Assistant string
	Org       string
	Project   string
	Workspace string
	SessionID string
}

type TaskExecutionResult struct {
	Task    *Task
	Session *Session
}

type plannedStep struct {
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

type taskExecutionMode struct {
	PendingApprovalID string
	StrictSteps       bool
}

type taskStageIndexes struct {
	analyze   int
	prepare   int
	execute   int
	verify    int
	summarize int
}

var ErrTaskWaitingApproval = errors.New("task waiting for approval")

const (
	taskEvidenceLimit = 200
	taskArtifactLimit = 64
)

func NewTaskManager(store *Store, sessions *SessionManager, runtimePool *RuntimePool, app taskAppInfo, planner taskPlanner, approvals *approvalManager, router *routing.Router, registry *plugin.Registry) *TaskManager {
	return &TaskManager{
		store:       store,
		sessions:    sessions,
		runtimePool: runtimePool,
		app:         app,
		planner:     planner,
		approvals:   approvals,
		router:      router,
		registry:    registry,
		nextID: func(prefix string) string {
			return uniqueID(prefix)
		},
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *TaskManager) Create(opts TaskCreateOptions) (*Task, error) {
	now := m.nowFunc()
	planSummary, stepDefs := m.planTask(context.Background(), strings.TrimSpace(opts.Input))
	task := &Task{
		ID:            m.nextID("task"),
		Title:         strings.TrimSpace(opts.Title),
		Input:         strings.TrimSpace(opts.Input),
		Status:        "queued",
		Assistant:     strings.TrimSpace(opts.Assistant),
		Org:           strings.TrimSpace(opts.Org),
		Project:       strings.TrimSpace(opts.Project),
		Workspace:     strings.TrimSpace(opts.Workspace),
		SessionID:     strings.TrimSpace(opts.SessionID),
		PlanSummary:   planSummary,
		CreatedAt:     now,
		LastUpdatedAt: now,
	}
	if task.Title == "" {
		task.Title = shortenTitle(task.Input)
	}
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "plan",
		Summary:   "Execution plan created.",
		Detail:    planSummary,
		StepIndex: 1,
		Status:    task.Status,
		Source:    "planner",
		Data: map[string]any{
			"step_count": len(stepDefs),
		},
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "queued",
		Summary:   "Task is queued and ready for execution.",
		StepIndex: 1,
		Status:    task.Status,
		Data: map[string]any{
			"step_count": len(stepDefs),
		},
	})
	if err := m.store.AppendTask(task); err != nil {
		return nil, err
	}
	steps := make([]*TaskStep, 0, len(stepDefs))
	for i, def := range stepDefs {
		step := &TaskStep{
			ID:        m.nextID("taskstep"),
			TaskID:    task.ID,
			Index:     i + 1,
			Title:     def.Title,
			Kind:      def.Kind,
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if i == 0 {
			step.Input = task.Input
		}
		steps = append(steps, step)
	}
	if err := m.store.ReplaceTaskSteps(task.ID, steps); err != nil {
		return nil, err
	}
	return task, nil
}

func (m *TaskManager) List() []*Task {
	return m.store.ListTasks()
}

func (m *TaskManager) Get(id string) (*Task, bool) {
	return m.store.GetTask(id)
}

func (m *TaskManager) Steps(taskID string) []*TaskStep {
	return m.store.ListTaskSteps(taskID)
}

func (m *TaskManager) MarkRejected(taskID string, stepIndex int, reason string) error {
	task, ok := m.store.GetTask(taskID)
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if strings.TrimSpace(reason) == "" {
		reason = "task execution rejected by approver"
	}
	task.Status = "failed"
	task.Error = reason
	task.CompletedAt = m.nowFunc().Format(time.RFC3339)
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "approval_rejected",
		Summary:   "Task execution was rejected during approval.",
		Detail:    reason,
		StepIndex: stepIndex,
		Status:    task.Status,
		Source:    "approval",
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "failed",
		Summary:   "Task stopped because approval was rejected.",
		StepIndex: stepIndex,
		Status:    task.Status,
		SessionID: task.SessionID,
		Data: map[string]any{
			"reason": reason,
		},
	})
	if err := m.persistTask(task); err != nil {
		return err
	}
	m.updateSessionPresence(task.SessionID, "idle", false)
	steps := m.store.ListTaskSteps(task.ID)
	failedStep := stepIndex
	if failedStep <= 0 {
		failedStep = 2
	}
	for i, step := range steps {
		status := "skipped"
		if step.Index == failedStep {
			status = "failed"
		} else if step.Index < failedStep {
			if step.Status == "completed" || (i == 0 && step.Status == "pending") {
				status = "completed"
			} else if strings.TrimSpace(step.Status) != "" && step.Status != "pending" {
				status = step.Status
			}
		}
		_ = m.setStepStatus(task.ID, step.Index, status, "", "", reason)
	}
	return nil
}

func (m *TaskManager) Execute(ctx context.Context, taskID string) (*TaskExecutionResult, error) {
	task, ok := m.store.GetTask(taskID)
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status == "completed" {
		return &TaskExecutionResult{Task: task}, nil
	}
	now := m.nowFunc()
	task.Status = "running"
	task.StartedAt = now.Format(time.RFC3339)
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "execution_started",
		Summary:   "Task execution started.",
		Detail:    task.PlanSummary,
		StepIndex: 2,
		Status:    task.Status,
		Source:    "task_manager",
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "execution",
		Summary:   "Task execution is in progress.",
		StepIndex: 2,
		Status:    task.Status,
		SessionID: task.SessionID,
	})
	if err := m.persistTask(task); err != nil {
		return nil, err
	}
	steps := m.store.ListTaskSteps(task.ID)
	if len(steps) == 0 {
		planSummary, planSteps := m.planTask(ctx, task.Input)
		task.PlanSummary = planSummary
		m.appendTaskEvidenceNoSave(task, TaskEvidence{
			Kind:      "plan_rebuilt",
			Summary:   "Execution plan was rebuilt before running.",
			Detail:    planSummary,
			StepIndex: 1,
			Status:    task.Status,
			Source:    "planner",
			Data: map[string]any{
				"step_count": len(planSteps),
			},
		})
		_ = m.persistTask(task)
		now = m.nowFunc()
		rebuilt := make([]*TaskStep, 0, len(planSteps))
		for i, def := range planSteps {
			rebuilt = append(rebuilt, &TaskStep{ID: m.nextID("taskstep"), TaskID: task.ID, Index: i + 1, Title: def.Title, Kind: def.Kind, Status: "pending", CreatedAt: now, UpdatedAt: now})
		}
		if len(rebuilt) > 0 {
			rebuilt[0].Input = task.Input
		}
		_ = m.store.ReplaceTaskSteps(task.ID, rebuilt)
		steps = rebuilt
	}

	session, err := m.ensureSession(task)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
	}
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "session_ready",
		Summary:   "Task is bound to an execution session.",
		StepIndex: 2,
		Status:    task.Status,
		Source:    "task_manager",
		Data: map[string]any{
			"session_id": session.ID,
		},
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "session",
		Summary:   "Task can resume from the linked execution session.",
		StepIndex: 2,
		Status:    task.Status,
		SessionID: session.ID,
	})
	_ = m.persistTask(task)

	if _, err := m.sessions.EnqueueTurn(session.ID); err == nil {
		session, _ = m.sessions.SetPresence(session.ID, "typing", true)
	}
	app, err := m.runtimePool.GetOrCreate(task.Assistant, task.Org, task.Project, task.Workspace)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
	}

	workflowMatches := m.resolveWorkflowMatches(ctx, task.Input, app.Plugins, app.LLM)
	if len(workflowMatches) > 0 {
		if alignedSteps, alignErr := m.adoptWorkflowMainPath(task, workflowMatches[0]); alignErr == nil {
			steps = alignedSteps
		}
	}
	stage := locateTaskStageIndexes(steps)
	if stage.analyze > 0 {
		_ = m.setStepStatus(task.ID, stage.analyze, "completed", task.Input, "Task request normalized and accepted.", "")
	}
	if stage.prepare > 0 {
		_ = m.setStepStatus(task.ID, stage.prepare, "running", task.Input, "", "")
	}

	execMode := m.executionMode(task)
	if execMode.StrictSteps && stage.execute > 0 {
		_ = m.setStepStatus(task.ID, stage.execute, "running", "", "Preparing strict step execution.", "")
	}

	if approvalErr := m.awaitApprovalsIfNeeded(task, session, app.Config, firstNonZero(stage.prepare, stage.execute, 2)); approvalErr != nil {
		if errors.Is(approvalErr, ErrTaskWaitingApproval) {
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return &TaskExecutionResult{Task: task, Session: session}, approvalErr
		}
		m.updateSessionPresence(session.ID, "idle", false)
		_ = m.failTask(task, approvalErr)
		return nil, approvalErr
	}
	task.PlanSummary = appendWorkflowPlanSummary(task.PlanSummary, workflowMatches)
	if len(workflowMatches) > 0 {
		toolNames := make([]string, 0, len(workflowMatches))
		for _, match := range workflowMatches {
			toolNames = append(toolNames, match.Workflow.ToolName)
		}
		m.appendTaskEvidenceNoSave(task, TaskEvidence{
			Kind:      "workflow_suggestions",
			Summary:   "Suggested app workflows were attached to the task.",
			Detail:    strings.Join(toolNames, ", "),
			StepIndex: 2,
			Status:    task.Status,
			Source:    "planner",
			Data: map[string]any{
				"matches": toolNames,
			},
		})
		m.appendTaskEvidenceNoSave(task, TaskEvidence{
			Kind:      "workflow_selected",
			Summary:   "Task main path was resolved to a workflow.",
			Detail:    workflowSelectionDetail(workflowMatches[0]),
			StepIndex: firstNonZero(stage.prepare, 2),
			Status:    task.Status,
			ToolName:  workflowMatches[0].Workflow.ToolName,
			Source:    "router",
		})
	}
	_ = m.persistTask(task)
	if stage.prepare > 0 && stage.prepare != stage.execute {
		_ = m.setStepStatus(task.ID, stage.prepare, "completed", task.Input, preparationStepOutput(workflowMatches), "")
	}
	if stage.execute > 0 {
		_ = m.setStepStatus(task.ID, stage.execute, "running", "", executionStageOutput(task, workflowMatches, nil), "")
	}
	app.Agent.SetHistory(m.historyWithWorkflowSuggestions(session.History, workflowMatches))
	execCtx := tools.WithBrowserSession(ctx, session.ID)
	execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: session.ID, Channel: "task"})
	execCtx = agent.WithToolApprovalHook(execCtx, m.toolApprovalHook(task, session, app.Config))
	execCtx = tools.WithToolApprovalHook(execCtx, m.protocolApprovalHook(task, session, app.Config))
	if task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		execCtx = appstate.WithDesktopPlanResumeState(execCtx, task.ExecutionState.DesktopPlan)
		m.appendTaskEvidenceNoSave(task, TaskEvidence{
			Kind:      "execution_resumed",
			Summary:   "Task resumed from a saved desktop workflow checkpoint.",
			Detail:    desktopPlanCheckpointDetail(task.ExecutionState.DesktopPlan),
			StepIndex: 3,
			Status:    task.Status,
			ToolName:  task.ExecutionState.DesktopPlan.ToolName,
			Source:    "desktop_plan",
			Data: map[string]any{
				"next_step":           task.ExecutionState.DesktopPlan.NextStep,
				"last_completed_step": task.ExecutionState.DesktopPlan.LastCompletedStep,
			},
		})
		_ = m.persistTask(task)
	}
	execCtx = appstate.WithDesktopPlanStateHook(execCtx, m.desktopPlanStateHook(task))
	response, err := app.Agent.Run(execCtx, task.Input)
	if freshTask, ok := m.store.GetTask(task.ID); ok && freshTask != nil {
		task = freshTask
	}
	toolActivities := app.Agent.GetLastToolActivities()
	m.recordTaskToolActivitiesNoSave(task, toolActivities)
	if len(toolActivities) > 0 {
		_ = m.persistTask(task)
	}
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return &TaskExecutionResult{Task: task, Session: session}, err
		}
		m.updateSessionPresence(session.ID, "idle", false)
		_ = m.failTask(task, err)
		return nil, err
	}
	updatedSession, err := m.sessions.AddExchange(session.ID, task.Input, response)
	if err != nil {
		m.updateSessionPresence(session.ID, "idle", false)
		_ = m.failTask(task, err)
		return nil, err
	}
	_, _ = m.sessions.SetPresence(updatedSession.ID, "idle", false)

	task.Result = response
	task.Status = "completed"
	task.CompletedAt = m.nowFunc().Format(time.RFC3339)
	steps = m.store.ListTaskSteps(task.ID)
	stage = locateTaskStageIndexes(steps)
	if stage.prepare > 0 && stage.prepare != stage.execute {
		_ = m.setStepStatus(task.ID, stage.prepare, "completed", task.Input, preparationStepOutput(workflowMatches), "")
	}
	if stage.execute > 0 {
		_ = m.setStepStatus(task.ID, stage.execute, "completed", "", executionStageOutput(task, workflowMatches, toolActivities), "")
	}
	verificationOutput, verificationObserved := verificationStageOutput(task, toolActivities)
	if stage.verify > 0 {
		_ = m.setStepStatus(task.ID, stage.verify, "completed", "", verificationOutput, "")
		m.appendTaskEvidenceNoSave(task, TaskEvidence{
			Kind:      verificationEvidenceKind(verificationObserved),
			Summary:   verificationEvidenceSummary(verificationObserved),
			Detail:    verificationOutput,
			StepIndex: stage.verify,
			Status:    task.Status,
			ToolName:  recoveryToolName(task),
			Source:    "verification",
		})
	}
	if stage.summarize > 0 {
		_ = m.setStepStatus(task.ID, stage.summarize, "completed", "", response, "")
	}
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "task_completed",
		Summary:   "Task completed with a recorded result.",
		Detail:    limitTaskText(response, 1200),
		StepIndex: firstNonZero(stage.summarize, len(steps)),
		Status:    task.Status,
		Source:    "assistant",
		Data: map[string]any{
			"session_id":          updatedSession.ID,
			"tool_activity_count": len(toolActivities),
			"artifact_count":      len(task.Artifacts),
		},
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "completed",
		Summary:   "Task completed. Review saved evidence and artifacts for verification.",
		StepIndex: firstNonZero(stage.summarize, len(steps)),
		Status:    task.Status,
		SessionID: updatedSession.ID,
		Data: map[string]any{
			"tool_activity_count": len(toolActivities),
			"artifact_count":      len(task.Artifacts),
		},
	})
	if err := m.persistTask(task); err != nil {
		return nil, err
	}

	return &TaskExecutionResult{Task: task, Session: updatedSession}, nil
}

func (m *TaskManager) executionMode(task *Task) taskExecutionMode {
	mode := taskExecutionMode{}
	if approval := m.findExecutionApproval(task.ID); approval != nil && approval.Status == "approved" {
		mode.PendingApprovalID = approval.ID
	}
	if task != nil && strings.Contains(strings.ToLower(task.PlanSummary), "inspect") {
		mode.StrictSteps = true
	}
	return mode
}

func (m *TaskManager) awaitApprovalsIfNeeded(task *Task, session *Session, cfg *config.Config, stepIndex int) error {
	if m.approvals == nil {
		return nil
	}
	if cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	if stepIndex <= 0 {
		stepIndex = 2
	}
	if existing := m.findExecutionApproval(task.ID); existing != nil {
		switch existing.Status {
		case "approved":
			task.Status = "running"
			m.appendTaskEvidenceNoSave(task, TaskEvidence{
				Kind:      "approval_granted",
				Summary:   "Execution approval was granted.",
				StepIndex: stepIndex,
				Status:    task.Status,
				Source:    "approval",
				Data: map[string]any{
					"approval_id": existing.ID,
				},
			})
			m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
				Kind:      "execution",
				Summary:   "Approval granted. Task execution can continue.",
				StepIndex: stepIndex,
				Status:    task.Status,
				SessionID: session.ID,
				Data: map[string]any{
					"approval_id": existing.ID,
				},
			})
			_ = m.persistTask(task)
			_ = m.setStepStatus(task.ID, stepIndex, "running", task.Input, "Approval granted. Executing planned work.", "")
			m.updateSessionPresence(session.ID, "typing", true)
			return nil
		case "rejected":
			return fmt.Errorf("task execution rejected by approver")
		case "pending":
			task.Status = "waiting_approval"
			m.appendTaskEvidenceNoSave(task, TaskEvidence{
				Kind:      "approval_waiting",
				Summary:   "Task is waiting for execution approval.",
				StepIndex: stepIndex,
				Status:    task.Status,
				Source:    "approval",
				Data: map[string]any{
					"approval_id": existing.ID,
					"scope":       "task_execution",
				},
			})
			m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
				Kind:      "approval",
				Summary:   "Awaiting approval before executing the task.",
				StepIndex: stepIndex,
				Status:    task.Status,
				SessionID: session.ID,
				Data: map[string]any{
					"approval_id": existing.ID,
					"scope":       "task_execution",
				},
			})
			_ = m.persistTask(task)
			_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", task.Input, "Awaiting approval before executing planned work.", "")
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return ErrTaskWaitingApproval
		}
	}
	payload := map[string]any{
		"task_title": task.Title,
		"input":      task.Input,
		"workspace":  task.Workspace,
		"assistant":  task.Assistant,
		"scope":      "task_execution",
	}
	approval, err := m.approvals.Request(task.ID, session.ID, stepIndex, "task_execution", "execute_task", payload)
	if err != nil {
		return err
	}
	task.Status = "waiting_approval"
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "approval_waiting",
		Summary:   "Task is waiting for execution approval.",
		StepIndex: stepIndex,
		Status:    task.Status,
		Source:    "approval",
		Data: map[string]any{
			"approval_id": approval.ID,
			"scope":       "task_execution",
		},
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "approval",
		Summary:   "Awaiting approval before executing the task.",
		StepIndex: stepIndex,
		Status:    task.Status,
		SessionID: session.ID,
		Data: map[string]any{
			"approval_id": approval.ID,
			"scope":       "task_execution",
		},
	})
	if err := m.persistTask(task); err != nil {
		return err
	}
	_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", task.Input, "Awaiting approval before executing planned work.", "")
	m.updateSessionPresence(session.ID, "waiting_approval", false)
	return ErrTaskWaitingApproval
}

func (m *TaskManager) toolApprovalHook(task *Task, session *Session, cfg *config.Config) agent.ToolApprovalHook {
	if m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	return func(ctx context.Context, tc agent.ToolCall) error {
		return m.requireToolApproval(task, session, cfg, tc.Name, tc.Args)
	}
}

func (m *TaskManager) protocolApprovalHook(task *Task, session *Session, cfg *config.Config) tools.ToolApprovalHook {
	if m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	return func(ctx context.Context, call tools.ToolApprovalCall) error {
		return m.requireToolApproval(task, session, cfg, call.Name, call.Args)
	}
}

func (m *TaskManager) requireToolApproval(task *Task, session *Session, cfg *config.Config, toolName string, args map[string]any) error {
	if !requiresToolApprovalName(toolName) {
		return nil
	}
	stepIndex := firstNonZero(m.executionStageIndexes(task.ID).execute, 3)
	signature := approvalSignature(toolName, "tool_call", args)
	for _, approval := range m.store.ListTaskApprovals(task.ID) {
		if approval.Signature != signature || approval.ToolName != toolName || approval.Action != "tool_call" {
			continue
		}
		switch approval.Status {
		case "approved":
			return nil
		case "rejected":
			return fmt.Errorf("tool call rejected: %s", toolName)
		case "pending":
			task.Status = "waiting_approval"
			m.appendTaskEvidenceNoSave(task, TaskEvidence{
				Kind:      "approval_waiting",
				Summary:   fmt.Sprintf("Task is waiting for approval to call %s.", toolName),
				StepIndex: stepIndex,
				Status:    task.Status,
				ToolName:  toolName,
				Source:    "approval",
				Data: map[string]any{
					"approval_id": approval.ID,
					"args":        cloneAnyMap(args),
				},
			})
			m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
				Kind:      "approval",
				Summary:   fmt.Sprintf("Awaiting approval for tool %s.", toolName),
				StepIndex: stepIndex,
				Status:    task.Status,
				SessionID: session.ID,
				ToolName:  toolName,
				Data: map[string]any{
					"approval_id": approval.ID,
					"args":        cloneAnyMap(args),
				},
			})
			_ = m.persistTask(task)
			_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", "", fmt.Sprintf("Awaiting approval for tool %s.", toolName), "")
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return ErrTaskWaitingApproval
		}
	}
	payload := map[string]any{
		"tool_name": toolName,
		"args":      args,
		"task_id":   task.ID,
		"workspace": task.Workspace,
	}
	approval, err := m.approvals.Request(task.ID, session.ID, stepIndex, toolName, "tool_call", payload)
	if err != nil {
		return err
	}
	task.Status = "waiting_approval"
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "approval_waiting",
		Summary:   fmt.Sprintf("Task is waiting for approval to call %s.", toolName),
		StepIndex: stepIndex,
		Status:    task.Status,
		ToolName:  toolName,
		Source:    "approval",
		Data: map[string]any{
			"approval_id": approval.ID,
			"args":        cloneAnyMap(args),
		},
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "approval",
		Summary:   fmt.Sprintf("Awaiting approval for tool %s.", toolName),
		StepIndex: stepIndex,
		Status:    task.Status,
		SessionID: session.ID,
		ToolName:  toolName,
		Data: map[string]any{
			"approval_id": approval.ID,
			"args":        cloneAnyMap(args),
		},
	})
	_ = m.persistTask(task)
	_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", "", fmt.Sprintf("Awaiting approval for tool %s.", toolName), "")
	m.updateSessionPresence(session.ID, "waiting_approval", false)
	return ErrTaskWaitingApproval
}

func requiresToolApproval(tc agent.ToolCall) bool {
	return requiresToolApprovalName(tc.Name)
}

func requiresToolApprovalName(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	switch name {
	case "run_command", "write_file", "browser_upload", "desktop_open", "desktop_type", "desktop_type_human", "desktop_hotkey", "desktop_clipboard_set", "desktop_clipboard_get", "desktop_paste", "desktop_click", "desktop_screenshot", "desktop_screenshot_window", "desktop_move", "desktop_double_click", "desktop_scroll", "desktop_drag", "desktop_wait", "desktop_list_windows", "desktop_wait_window", "desktop_focus_window", "desktop_inspect_ui", "desktop_invoke_ui", "desktop_set_value_ui", "desktop_resolve_target", "desktop_activate_target", "desktop_set_target_value", "desktop_match_image", "desktop_click_image", "desktop_wait_image", "desktop_ocr", "desktop_verify_text", "desktop_find_text", "desktop_click_text", "desktop_wait_text", "desktop_plan":
		return true
	default:
		return false
	}
}

func (m *TaskManager) findExecutionApproval(taskID string) *Approval {
	approvals := m.store.ListApprovals("")
	for _, approval := range approvals {
		if approval.TaskID == taskID && approval.ToolName == "task_execution" && approval.Action == "execute_task" {
			return approval
		}
	}
	return nil
}

func (m *TaskManager) ensureSession(task *Task) (*Session, error) {
	if strings.TrimSpace(task.SessionID) != "" {
		session, ok := m.sessions.Get(task.SessionID)
		if ok {
			return session, nil
		}
	}
	session, err := m.sessions.CreateWithOptions(SessionCreateOptions{
		Title:     task.Title,
		AgentName: firstNonEmpty(task.Assistant, m.app.Name),
		Org:       task.Org,
		Project:   task.Project,
		Workspace: task.Workspace,
		QueueMode: "fifo",
	})
	if err != nil {
		return nil, err
	}
	task.SessionID = session.ID
	if err := m.persistTask(task); err != nil {
		return nil, err
	}
	return session, nil
}

func (m *TaskManager) failTask(task *Task, err error) error {
	task.Status = "failed"
	task.Error = err.Error()
	task.CompletedAt = m.nowFunc().Format(time.RFC3339)
	steps := m.store.ListTaskSteps(task.ID)
	stage := locateTaskStageIndexes(steps)
	failedIndex := firstNonZero(stage.execute, stage.prepare, 2)
	if failedIndex > 0 {
		_ = m.setStepStatus(task.ID, failedIndex, "failed", task.Input, "", err.Error())
	}
	for _, step := range steps {
		if step.Index <= failedIndex {
			continue
		}
		_ = m.setStepStatus(task.ID, step.Index, "skipped", "", "", err.Error())
	}
	recoveryData := map[string]any{
		"error": err.Error(),
	}
	if task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		recoveryData["desktop_plan_tool"] = task.ExecutionState.DesktopPlan.ToolName
		recoveryData["desktop_plan_next_step"] = task.ExecutionState.DesktopPlan.NextStep
		recoveryData["desktop_plan_last_completed_step"] = task.ExecutionState.DesktopPlan.LastCompletedStep
	}
	m.appendTaskEvidenceNoSave(task, TaskEvidence{
		Kind:      "task_failed",
		Summary:   "Task execution failed.",
		Detail:    err.Error(),
		StepIndex: failedIndex,
		Status:    task.Status,
		Source:    "task_manager",
		Data:      recoveryData,
	})
	m.setTaskRecoveryPointNoSave(task, &TaskRecoveryPoint{
		Kind:      "failed",
		Summary:   "Task stopped after an error. Review saved evidence and checkpoints before retrying.",
		StepIndex: failedIndex,
		Status:    task.Status,
		SessionID: task.SessionID,
		ToolName:  recoveryToolName(task),
		Data:      recoveryData,
	})
	return m.persistTask(task)
}

func (m *TaskManager) setStepStatus(taskID string, index int, status string, input string, output string, stepErr string) error {
	steps := m.store.ListTaskSteps(taskID)
	for _, step := range steps {
		if step.Index != index {
			continue
		}
		if input != "" {
			step.Input = input
		}
		if output != "" {
			step.Output = output
		}
		step.Error = stepErr
		step.Status = status
		step.UpdatedAt = m.nowFunc()
		return m.store.UpdateTaskStep(step)
	}
	return nil
}

func (m *TaskManager) executionStageIndexes(taskID string) taskStageIndexes {
	return locateTaskStageIndexes(m.store.ListTaskSteps(taskID))
}

func locateTaskStageIndexes(steps []*TaskStep) taskStageIndexes {
	indexes := taskStageIndexes{
		analyze:   firstTaskStepIndexByKinds(steps, "analyze"),
		prepare:   firstTaskStepIndexByKinds(steps, "workflow", "inspect"),
		execute:   firstTaskStepIndexByKinds(steps, "desktop_plan", "execute"),
		verify:    firstTaskStepIndexByKinds(steps, "verify", "verification"),
		summarize: firstTaskStepIndexByKinds(steps, "summarize"),
	}
	if indexes.analyze == 0 && len(steps) > 0 {
		indexes.analyze = steps[0].Index
	}
	if indexes.prepare == 0 {
		indexes.prepare = firstTaskStepIndexByKinds(steps, "execute")
	}
	if indexes.execute == 0 {
		indexes.execute = indexes.prepare
	}
	return indexes
}

func firstTaskStepIndexByKinds(steps []*TaskStep, kinds ...string) int {
	for _, step := range steps {
		if taskStepKindMatches(step.Kind, kinds...) {
			return step.Index
		}
	}
	return 0
}

func taskStepKindMatches(kind string, kinds ...string) bool {
	current := normalizeTaskStepKind(kind)
	for _, candidate := range kinds {
		if current == normalizeTaskStepKind(candidate) {
			return true
		}
	}
	return false
}

func normalizeTaskStepKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	replacer := strings.NewReplacer(" ", "_", "-", "_")
	kind = replacer.Replace(kind)
	switch kind {
	case "verification":
		return "verify"
	default:
		return kind
	}
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (m *TaskManager) adoptWorkflowMainPath(task *Task, match plugin.AppWorkflowMatch) ([]*TaskStep, error) {
	if task == nil {
		return nil, nil
	}
	current := m.store.ListTaskSteps(task.ID)
	if len(current) == 0 || !canAdoptWorkflowMainPath(current) || hasExplicitWorkflowMainPath(current) {
		return current, nil
	}
	now := m.nowFunc()
	workflowTitle := "Resolve workflow: " + match.Workflow.ToolName
	if strings.TrimSpace(match.Workflow.Name) != "" {
		workflowTitle = "Resolve workflow: " + strings.TrimSpace(match.Workflow.Name)
	}
	desktopTitle := "Execute desktop plan"
	if appName := strings.TrimSpace(match.Workflow.App); appName != "" {
		desktopTitle = "Execute desktop plan: " + appName
	}
	steps := []*TaskStep{
		{ID: m.nextID("taskstep"), TaskID: task.ID, Index: 1, Title: "Analyze the request", Kind: "analyze", Status: "pending", Input: task.Input, CreatedAt: now, UpdatedAt: now},
		{ID: m.nextID("taskstep"), TaskID: task.ID, Index: 2, Title: workflowTitle, Kind: "workflow", ToolName: match.Workflow.ToolName, Status: "pending", CreatedAt: now, UpdatedAt: now},
		{ID: m.nextID("taskstep"), TaskID: task.ID, Index: 3, Title: desktopTitle, Kind: "desktop_plan", ToolName: "desktop_plan", Status: "pending", CreatedAt: now, UpdatedAt: now},
		{ID: m.nextID("taskstep"), TaskID: task.ID, Index: 4, Title: "Verify the requested outcome with observable evidence", Kind: "verify", Status: "pending", CreatedAt: now, UpdatedAt: now},
		{ID: m.nextID("taskstep"), TaskID: task.ID, Index: 5, Title: "Summarize the final result", Kind: "summarize", Status: "pending", CreatedAt: now, UpdatedAt: now},
	}
	if err := m.store.ReplaceTaskSteps(task.ID, steps); err != nil {
		return current, err
	}
	return steps, nil
}

func canAdoptWorkflowMainPath(steps []*TaskStep) bool {
	for _, step := range steps {
		status := strings.TrimSpace(strings.ToLower(step.Status))
		if status == "" || status == "pending" {
			continue
		}
		return false
	}
	return true
}

func hasExplicitWorkflowMainPath(steps []*TaskStep) bool {
	for _, step := range steps {
		if taskStepKindMatches(step.Kind, "workflow", "desktop_plan") {
			return true
		}
	}
	return false
}

func workflowSelectionDetail(match plugin.AppWorkflowMatch) string {
	detail := fmt.Sprintf("%s -> %s", match.Workflow.App, match.Workflow.ToolName)
	if match.Pairing != nil && strings.TrimSpace(match.Pairing.Name) != "" {
		detail += " @" + strings.TrimSpace(match.Pairing.Name)
	}
	if strings.TrimSpace(match.Reason) != "" {
		detail += " | " + strings.TrimSpace(match.Reason)
	}
	return detail
}

func preparationStepOutput(matches []plugin.AppWorkflowMatch) string {
	if len(matches) == 0 {
		return "Runtime and workspace context are ready for execution."
	}
	return "Workflow resolved: " + workflowSelectionDetail(matches[0])
}

func executionStageOutput(task *Task, matches []plugin.AppWorkflowMatch, activities []agent.ToolActivity) string {
	if task != nil && task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		state := task.ExecutionState.DesktopPlan
		if result := strings.TrimSpace(state.Result); result != "" {
			return result
		}
		return desktopPlanCheckpointDetail(state)
	}
	if len(matches) > 0 {
		return "Workflow-driven execution completed."
	}
	if len(activities) > 0 {
		names := make([]string, 0, len(activities))
		for _, activity := range activities {
			if name := strings.TrimSpace(activity.ToolName); name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return "Execution completed with tools: " + strings.Join(uniqueTaskStrings(names), ", ")
		}
	}
	return "Execution completed using the current runtime."
}

func verificationStageOutput(task *Task, activities []agent.ToolActivity) (string, bool) {
	if task != nil && task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		state := task.ExecutionState.DesktopPlan
		if desktopPlanHasExplicitVerification(state) {
			return verificationOutputFromDesktopPlan(state), true
		}
	}
	toolsUsed := observedVerificationTools(activities)
	if len(toolsUsed) > 0 {
		return "Observed verification tool calls: " + strings.Join(toolsUsed, ", "), true
	}
	return "No explicit verification tool was observed; completion relies on recorded tool outputs and the final result.", false
}

func verificationOutputFromDesktopPlan(state *appstate.DesktopPlanExecutionState) string {
	if state == nil {
		return ""
	}
	total := 0
	passed := 0
	for _, step := range state.Steps {
		if step.HasVerify {
			total++
		}
		if step.Verified {
			passed++
		}
	}
	if total == 0 {
		return "Desktop plan completed without explicit verification steps."
	}
	return fmt.Sprintf("Desktop plan verified %d of %d step(s).", passed, total)
}

func desktopPlanHasExplicitVerification(state *appstate.DesktopPlanExecutionState) bool {
	if state == nil {
		return false
	}
	for _, step := range state.Steps {
		if step.Verified {
			return true
		}
	}
	return false
}

func observedVerificationTools(activities []agent.ToolActivity) []string {
	if len(activities) == 0 {
		return nil
	}
	items := make([]string, 0, len(activities))
	for _, activity := range activities {
		switch strings.TrimSpace(strings.ToLower(activity.ToolName)) {
		case "desktop_verify_text", "desktop_wait_text", "desktop_find_text", "desktop_resolve_target", "desktop_ocr", "read_file", "browser_snapshot", "browser_screenshot":
			items = append(items, activity.ToolName)
		}
	}
	return uniqueTaskStrings(items)
}

func verificationEvidenceKind(observed bool) string {
	if observed {
		return "verification_completed"
	}
	return "verification_gap"
}

func verificationEvidenceSummary(observed bool) string {
	if observed {
		return "Task outcome was checked with observable verification."
	}
	return "Task completed without an explicit verification tool signal."
}

func uniqueTaskStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func (m *TaskManager) updateSessionPresence(sessionID string, presence string, typing bool) {
	if m.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_, _ = m.sessions.SetPresence(sessionID, presence, typing)
}

func (m *TaskManager) desktopPlanStateHook(task *Task) appstate.DesktopPlanStateHook {
	if m == nil || task == nil {
		return nil
	}
	return func(ctx context.Context, state appstate.DesktopPlanExecutionState) {
		freshTask, ok := m.store.GetTask(task.ID)
		if ok && freshTask != nil {
			task = freshTask
		}
		if task.ExecutionState == nil {
			task.ExecutionState = &TaskExecutionState{}
		}
		previous := task.ExecutionState.DesktopPlan
		task.ExecutionState.DesktopPlan = appstate.CloneDesktopPlanExecutionState(&state)
		stage := m.executionStageIndexes(task.ID)
		executeIndex := firstNonZero(stage.execute, 3)
		if shouldRecordDesktopPlanCheckpoint(previous, &state) {
			m.appendTaskEvidenceNoSave(task, TaskEvidence{
				Kind:      "desktop_checkpoint",
				Summary:   "Saved desktop workflow checkpoint.",
				Detail:    desktopPlanCheckpointDetail(&state),
				StepIndex: executeIndex,
				Status:    firstNonEmpty(state.Status, task.Status),
				ToolName:  state.ToolName,
				Source:    "desktop_plan",
				Data: map[string]any{
					"current_step":        state.CurrentStep,
					"next_step":           state.NextStep,
					"last_completed_step": state.LastCompletedStep,
					"total_steps":         state.TotalSteps,
				},
			})
		}
		switch strings.ToLower(strings.TrimSpace(state.Status)) {
		case "pending_approval", "waiting_approval":
			_ = m.setStepStatus(task.ID, executeIndex, "waiting_approval", "", desktopPlanCheckpointDetail(&state), "")
		case "running", "resuming":
			_ = m.setStepStatus(task.ID, executeIndex, "running", "", desktopPlanCheckpointDetail(&state), "")
		case "completed":
			_ = m.setStepStatus(task.ID, executeIndex, "completed", "", executionStageOutput(task, nil, nil), "")
			if stage.verify > 0 && desktopPlanHasExplicitVerification(&state) {
				_ = m.setStepStatus(task.ID, stage.verify, "completed", "", verificationOutputFromDesktopPlan(&state), "")
			}
		case "failed", "interrupted":
			_ = m.setStepStatus(task.ID, executeIndex, "failed", "", "", firstNonEmpty(state.LastError, desktopPlanCheckpointDetail(&state)))
		}
		m.setTaskRecoveryPointNoSave(task, desktopPlanRecoveryPoint(task, &state))
		_ = m.persistTask(task)
	}
}

func (m *TaskManager) persistTask(task *Task) error {
	if task == nil {
		return nil
	}
	task.LastUpdatedAt = m.nowFunc()
	return m.store.UpdateTask(task)
}

func (m *TaskManager) appendTaskEvidenceNoSave(task *Task, evidence TaskEvidence) {
	if task == nil {
		return
	}
	evidence.Kind = strings.TrimSpace(evidence.Kind)
	if evidence.Kind == "" {
		evidence.Kind = "note"
	}
	evidence.Summary = limitTaskText(evidence.Summary, 240)
	evidence.Detail = limitTaskText(evidence.Detail, 1200)
	if strings.TrimSpace(evidence.ID) == "" {
		evidence.ID = m.nextID("evidence")
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = m.nowFunc()
	}
	evidence.Data = cloneAnyMap(evidence.Data)
	task.Evidence = append(task.Evidence, &evidence)
	if len(task.Evidence) > taskEvidenceLimit {
		task.Evidence = task.Evidence[len(task.Evidence)-taskEvidenceLimit:]
	}
}

func (m *TaskManager) setTaskRecoveryPointNoSave(task *Task, point *TaskRecoveryPoint) {
	if task == nil {
		return
	}
	if point == nil {
		task.RecoveryPoint = nil
		return
	}
	clone := cloneTaskRecoveryPoint(point)
	clone.Kind = strings.TrimSpace(clone.Kind)
	if clone.Kind == "" {
		clone.Kind = "task"
	}
	clone.Summary = limitTaskText(clone.Summary, 240)
	if clone.UpdatedAt.IsZero() {
		clone.UpdatedAt = m.nowFunc()
	}
	task.RecoveryPoint = clone
}

func (m *TaskManager) appendTaskArtifactNoSave(task *Task, artifact TaskArtifact) {
	if task == nil {
		return
	}
	artifact.Kind = strings.TrimSpace(artifact.Kind)
	if artifact.Kind == "" {
		artifact.Kind = "file"
	}
	artifact.Label = limitTaskText(strings.TrimSpace(artifact.Label), 160)
	artifact.Path = strings.TrimSpace(artifact.Path)
	artifact.Description = limitTaskText(strings.TrimSpace(artifact.Description), 320)
	if strings.TrimSpace(artifact.ID) == "" {
		artifact.ID = m.nextID("artifact")
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = m.nowFunc()
	}
	artifact.Meta = cloneAnyMap(artifact.Meta)
	for _, existing := range task.Artifacts {
		if existing == nil {
			continue
		}
		if existing.Kind == artifact.Kind && existing.ToolName == artifact.ToolName && existing.Path == artifact.Path && existing.Label == artifact.Label {
			return
		}
	}
	task.Artifacts = append(task.Artifacts, &artifact)
	if len(task.Artifacts) > taskArtifactLimit {
		task.Artifacts = task.Artifacts[len(task.Artifacts)-taskArtifactLimit:]
	}
}

func (m *TaskManager) recordTaskToolActivitiesNoSave(task *Task, activities []agent.ToolActivity) {
	if task == nil {
		return
	}
	for _, activity := range activities {
		status := "completed"
		summary := fmt.Sprintf("Tool %s executed.", activity.ToolName)
		detail := limitTaskText(activity.Result, 800)
		if strings.TrimSpace(activity.Error) != "" {
			status = "failed"
			summary = fmt.Sprintf("Tool %s failed.", activity.ToolName)
			detail = limitTaskText(activity.Error, 800)
		}
		m.appendTaskEvidenceNoSave(task, TaskEvidence{
			Kind:      "tool_activity",
			Summary:   summary,
			Detail:    detail,
			StepIndex: 3,
			Status:    status,
			ToolName:  activity.ToolName,
			Source:    "agent",
			Data: map[string]any{
				"args": cloneAnyMap(activity.Args),
			},
		})
		for _, artifact := range inferTaskArtifacts(activity) {
			m.appendTaskArtifactNoSave(task, artifact)
		}
	}
}

func inferTaskArtifacts(activity agent.ToolActivity) []TaskArtifact {
	items := make([]TaskArtifact, 0)
	for key, value := range activity.Args {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" || !looksLikeArtifactKey(key) {
			continue
		}
		items = append(items, TaskArtifact{
			Kind:        inferArtifactKind(activity.ToolName, key, text),
			Label:       fmt.Sprintf("%s:%s", activity.ToolName, key),
			Path:        text,
			ToolName:    activity.ToolName,
			Description: "Observed from tool arguments during task execution.",
			Meta: map[string]any{
				"arg": key,
			},
		})
	}
	return items
}

func looksLikeArtifactKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "path") ||
		strings.Contains(key, "file") ||
		strings.Contains(key, "output") ||
		strings.Contains(key, "download") ||
		strings.Contains(key, "save") ||
		strings.Contains(key, "export") ||
		strings.Contains(key, "destination") ||
		strings.Contains(key, "screenshot")
}

func inferArtifactKind(toolName string, key string, value string) string {
	lowerTool := strings.ToLower(strings.TrimSpace(toolName))
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lowerTool, "screenshot") || strings.Contains(lowerKey, "screenshot"):
		return "screenshot"
	case strings.Contains(lowerTool, "pdf") || strings.HasSuffix(lowerValue, ".pdf"):
		return "pdf"
	case strings.Contains(lowerTool, "download") || strings.Contains(lowerKey, "download"):
		return "download"
	default:
		return "file"
	}
}

func desktopPlanRecoveryPoint(task *Task, state *appstate.DesktopPlanExecutionState) *TaskRecoveryPoint {
	if state == nil {
		return nil
	}
	return &TaskRecoveryPoint{
		Kind:      "desktop_plan",
		Summary:   "Resume from the saved desktop workflow checkpoint.",
		StepIndex: 3,
		Status:    firstNonEmpty(state.Status, taskStatus(task)),
		SessionID: taskSessionID(task),
		ToolName:  state.ToolName,
		Data: map[string]any{
			"current_step":        state.CurrentStep,
			"next_step":           state.NextStep,
			"last_completed_step": state.LastCompletedStep,
			"total_steps":         state.TotalSteps,
			"workflow":            state.Workflow,
			"action":              state.Action,
		},
	}
}

func shouldRecordDesktopPlanCheckpoint(previous *appstate.DesktopPlanExecutionState, current *appstate.DesktopPlanExecutionState) bool {
	if current == nil {
		return false
	}
	if previous == nil {
		return true
	}
	return previous.Status != current.Status ||
		previous.CurrentStep != current.CurrentStep ||
		previous.NextStep != current.NextStep ||
		previous.LastCompletedStep != current.LastCompletedStep ||
		previous.LastError != current.LastError ||
		previous.LastOutput != current.LastOutput
}

func desktopPlanCheckpointDetail(state *appstate.DesktopPlanExecutionState) string {
	if state == nil {
		return ""
	}
	parts := []string{}
	if state.ToolName != "" {
		parts = append(parts, "tool="+state.ToolName)
	}
	if state.Status != "" {
		parts = append(parts, "status="+state.Status)
	}
	if state.CurrentStep > 0 || state.TotalSteps > 0 {
		parts = append(parts, fmt.Sprintf("step=%d/%d", state.CurrentStep, state.TotalSteps))
	}
	if state.NextStep > 0 {
		parts = append(parts, fmt.Sprintf("next=%d", state.NextStep))
	}
	if state.LastCompletedStep > 0 {
		parts = append(parts, fmt.Sprintf("last_completed=%d", state.LastCompletedStep))
	}
	totalVerifies := 0
	passedVerifies := 0
	for _, step := range state.Steps {
		if step.HasVerify {
			totalVerifies++
		}
		if step.Verified {
			passedVerifies++
		}
	}
	if totalVerifies > 0 {
		parts = append(parts, fmt.Sprintf("verified=%d/%d", passedVerifies, totalVerifies))
	}
	if text := firstNonEmpty(state.Summary, state.Result, state.LastOutput, state.LastError); strings.TrimSpace(text) != "" {
		parts = append(parts, limitTaskText(text, 240))
	}
	return strings.Join(parts, " | ")
}

func recoveryToolName(task *Task) string {
	if task == nil || task.ExecutionState == nil || task.ExecutionState.DesktopPlan == nil {
		return ""
	}
	return task.ExecutionState.DesktopPlan.ToolName
}

func taskSessionID(task *Task) string {
	if task == nil {
		return ""
	}
	return task.SessionID
}

func taskStatus(task *Task) string {
	if task == nil {
		return ""
	}
	return task.Status
}

func limitTaskText(input string, max int) string {
	trimmed := strings.TrimSpace(input)
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	if max <= 3 {
		return trimmed[:max]
	}
	return trimmed[:max-3] + "..."
}

func (m *TaskManager) routeWithLowToken(ctx context.Context, input string) *routing.RouteResult {
	intent := routing.TaskIntent{
		ID:        m.nextID("intent"),
		Input:     input,
		CreatedAt: time.Now().UTC(),
	}
	result, err := m.router.RouteTask(ctx, intent)
	if err == nil && result != nil && result.Confidence >= 0.7 {
		return result
	}
	return nil
}

func (m *TaskManager) buildPlanFromRouteResult(result *routing.RouteResult) (string, []plannedStep) {
	var steps []plannedStep
	summary := result.Explanation

	switch result.Mode {
	case "workflow":
		steps = []plannedStep{
			{Title: "解析工作流请求: " + result.Workflow, Kind: "analyze"},
			{Title: "执行工作流: " + result.Workflow, Kind: "execute"},
			{Title: "验证工作流执行结果", Kind: "verify"},
			{Title: "总结执行结果", Kind: "summarize"},
		}
	case "app-action":
		steps = []plannedStep{
			{Title: "启动应用: " + result.App, Kind: "execute"},
			{Title: "执行应用操作", Kind: "execute"},
			{Title: "验证操作结果", Kind: "verify"},
			{Title: "总结执行结果", Kind: "summarize"},
		}
	case "tool-chain":
		steps = []plannedStep{
			{Title: "分析任务需求", Kind: "analyze"},
			{Title: "执行工具链", Kind: "execute"},
			{Title: "验证执行结果", Kind: "verify"},
			{Title: "总结执行结果", Kind: "summarize"},
		}
	default:
		steps = []plannedStep{
			{Title: "分析请求: " + result.Explanation, Kind: "analyze"},
			{Title: "执行任务", Kind: "execute"},
			{Title: "验证执行结果", Kind: "verify"},
			{Title: "总结执行结果", Kind: "summarize"},
		}
	}

	if result.RequiresApproval {
		steps = append([]plannedStep{{Title: "等待用户审批", Kind: "analyze"}}, steps...)
	}

	return summary, steps
}

func (m *TaskManager) planTask(ctx context.Context, input string) (string, []plannedStep) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultPlan(input)
	}

	// First try low-token routing (L0/L1 path) for common tasks
	if m.router != nil && m.registry != nil {
		routeResult := m.routeWithLowToken(ctx, trimmed)
		if routeResult != nil {
			return m.buildPlanFromRouteResult(routeResult)
		}
	}

	if m.planner == nil {
		return defaultPlan(trimmed)
	}
	messages := []llm.Message{
		{Role: "system", Content: "You generate concise execution plans for local AI tasks. Return JSON only with fields summary and steps. Each step must contain title and kind. Use 4 to 6 steps. kinds can be analyze, inspect, execute, verify, summarize. Plans must include a verify step before summarize and should assume the runtime will observe real state before declaring success."},
		{Role: "user", Content: fmt.Sprintf("Plan this task for execution in a local assistant runtime: %s", trimmed)},
	}
	resp, err := m.planner.Chat(ctx, messages, nil)
	if err != nil {
		return defaultPlan(trimmed)
	}
	var payload struct {
		Summary string        `json:"summary"`
		Steps   []plannedStep `json:"steps"`
	}
	raw := strings.TrimSpace(resp.Content)
	if raw == "" {
		return defaultPlan(trimmed)
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &payload); err != nil {
		return defaultPlan(trimmed)
	}
	payload.Summary = strings.TrimSpace(payload.Summary)
	if payload.Summary == "" || len(payload.Steps) == 0 {
		return defaultPlan(trimmed)
	}
	steps := make([]plannedStep, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		title := strings.TrimSpace(step.Title)
		kind := normalizeStepKind(step.Kind)
		if title == "" {
			continue
		}
		steps = append(steps, plannedStep{Title: title, Kind: kind})
	}
	if len(steps) == 0 {
		return defaultPlan(trimmed)
	}
	steps = ensureRequiredPlanSteps(steps)
	return payload.Summary, steps
}

func defaultPlan(input string) (string, []plannedStep) {
	trimmed := strings.TrimSpace(input)
	summary := "Analyze the request, inspect the workspace if needed, execute the task, verify the observable outcome, and summarize the result."
	if trimmed != "" {
		summary = fmt.Sprintf("Analyze the request (%s), inspect the workspace if needed, execute the task, verify the observable outcome, and summarize the result.", shortenTitle(trimmed))
	}
	return summary, []plannedStep{
		{Title: "Analyze the request", Kind: "analyze"},
		{Title: "Inspect relevant files or workspace context", Kind: "inspect"},
		{Title: "Execute the requested work", Kind: "execute"},
		{Title: "Verify the requested outcome with observable evidence", Kind: "verify"},
		{Title: "Summarize the final result", Kind: "summarize"},
	}
}

func normalizeStepKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	switch kind {
	case "analyze", "inspect", "execute", "verify", "summarize":
		return kind
	default:
		return "execute"
	}
}

func ensureRequiredPlanSteps(steps []plannedStep) []plannedStep {
	result := append([]plannedStep(nil), steps...)
	if !planHasKind(result, "verify") {
		verifyStep := plannedStep{Title: "Verify the requested outcome with observable evidence", Kind: "verify"}
		if len(result) > 0 && result[len(result)-1].Kind == "summarize" {
			result = append(result[:len(result)-1], append([]plannedStep{verifyStep}, result[len(result)-1:]...)...)
		} else {
			result = append(result, verifyStep)
		}
	}
	if !planHasKind(result, "summarize") {
		result = append(result, plannedStep{Title: "Summarize the final result", Kind: "summarize"})
	}
	return result
}

func planHasKind(steps []plannedStep, kind string) bool {
	kind = normalizeStepKind(kind)
	for _, step := range steps {
		if normalizeStepKind(step.Kind) == kind {
			return true
		}
	}
	return false
}

func extractJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```") {
		parts := strings.Split(input, "```")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "json") {
				part = strings.TrimSpace(strings.TrimPrefix(part, "json"))
			}
			if strings.HasPrefix(part, "{") {
				return part
			}
		}
	}
	return input
}

func (m *TaskManager) resolveWorkflowMatches(ctx context.Context, input string, registry *plugin.Registry, planner taskPlanner) []plugin.AppWorkflowMatch {
	if m.router != nil {
		if strings.TrimSpace(m.app.ConfigPath) != "" {
			if store, err := appstate.NewStore(m.app.ConfigPath); err == nil {
				return m.router.ResolveWorkflowMatchesWithPairings(ctx, input, 6, convertToAnyPairings(store.ListPairings()))
			}
		}
		return m.router.ResolveWorkflowMatches(ctx, input, 6)
	}
	if registry == nil || strings.TrimSpace(input) == "" {
		return nil
	}
	candidates := registry.ResolveWorkflowMatches(input, 6)
	if strings.TrimSpace(m.app.ConfigPath) != "" {
		if store, err := appstate.NewStore(m.app.ConfigPath); err == nil {
			candidates = registry.ResolveWorkflowMatchesWithPairings(input, 6, store.ListPairings())
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	if planner == nil || len(candidates) <= 1 {
		return trimWorkflowMatches(candidates, 3)
	}
	selected, ok := rerankWorkflowMatchesWithPlanner(ctx, planner, input, candidates)
	if ok && len(selected) > 0 {
		return selected
	}
	return trimWorkflowMatches(candidates, 3)
}

func convertToAnyPairings(pairings []*apps.Pairing) []any {
	if pairings == nil {
		return nil
	}
	result := make([]any, len(pairings))
	for i, p := range pairings {
		result[i] = p
	}
	return result
}

func rerankWorkflowMatchesWithPlanner(ctx context.Context, planner taskPlanner, input string, candidates []plugin.AppWorkflowMatch) ([]plugin.AppWorkflowMatch, bool) {
	if planner == nil || len(candidates) == 0 {
		return nil, false
	}
	lines := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		line := fmt.Sprintf("- %s | app=%s | workflow=%s | action=%s | tags=%s | desc=%s",
			candidate.Workflow.ToolName,
			candidate.Workflow.App,
			candidate.Workflow.Name,
			candidate.Workflow.Action,
			strings.Join(candidate.Workflow.Tags, ", "),
			candidate.Workflow.Description,
		)
		if candidate.Pairing != nil {
			line += fmt.Sprintf(" | pairing=%s", candidate.Pairing.Name)
			if strings.TrimSpace(candidate.Pairing.Binding) != "" {
				line += fmt.Sprintf(" | binding=%s", strings.TrimSpace(candidate.Pairing.Binding))
			}
		}
		lines = append(lines, line)
	}
	messages := []llm.Message{
		{Role: "system", Content: "You rank candidate app workflows for a local assistant. Return JSON only with {\"matches\":[{\"tool_name\":\"...\",\"reason\":\"...\"}]}. Choose at most 3 tool names. Only choose from the listed tool names."},
		{Role: "user", Content: fmt.Sprintf("User request:\n%s\n\nCandidate workflows:\n%s", strings.TrimSpace(input), strings.Join(lines, "\n"))},
	}
	resp, err := planner.Chat(ctx, messages, nil)
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, false
	}
	var payload struct {
		Matches []struct {
			ToolName string `json:"tool_name"`
			Reason   string `json:"reason"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &payload); err != nil {
		return nil, false
	}
	byTool := make(map[string]plugin.AppWorkflowMatch, len(candidates))
	for _, candidate := range candidates {
		byTool[candidate.Workflow.ToolName] = candidate
	}
	selected := make([]plugin.AppWorkflowMatch, 0, len(payload.Matches))
	for _, item := range payload.Matches {
		match, ok := byTool[strings.TrimSpace(item.ToolName)]
		if !ok {
			continue
		}
		if strings.TrimSpace(item.Reason) != "" {
			match.Reason = strings.TrimSpace(item.Reason)
		}
		selected = append(selected, match)
		if len(selected) >= 3 {
			break
		}
	}
	if len(selected) == 0 {
		return nil, false
	}
	return selected, true
}

func trimWorkflowMatches(matches []plugin.AppWorkflowMatch, limit int) []plugin.AppWorkflowMatch {
	if len(matches) == 0 {
		return nil
	}
	if limit <= 0 || len(matches) <= limit {
		return append([]plugin.AppWorkflowMatch(nil), matches...)
	}
	return append([]plugin.AppWorkflowMatch(nil), matches[:limit]...)
}

func (m *TaskManager) historyWithWorkflowSuggestions(history []prompt.Message, matches []plugin.AppWorkflowMatch) []prompt.Message {
	if len(matches) == 0 {
		return history
	}
	items := append([]prompt.Message(nil), history...)
	items = append(items, prompt.Message{
		Role:    "system",
		Content: buildWorkflowGuidance(matches),
	})
	return items
}

func buildWorkflowGuidance(matches []plugin.AppWorkflowMatch) string {
	lines := []string{
		"Suggested app workflows are available for this task. Prefer these higher-level workflow tools before using low-level desktop tools directly when they fit the request:",
		"If no workflow fits exactly, prefer target-based desktop tools such as desktop_resolve_target, desktop_activate_target, and desktop_set_target_value before raw coordinate clicks.",
	}
	for _, match := range matches {
		line := fmt.Sprintf("- %s: %s (%s / %s)", match.Workflow.ToolName, firstNonEmpty(match.Workflow.Description, match.Workflow.Name), match.Workflow.App, match.Workflow.Action)
		if match.Pairing != nil {
			line += fmt.Sprintf(" | pairing=%s", match.Pairing.Name)
			if strings.TrimSpace(match.Pairing.Binding) != "" {
				line += fmt.Sprintf(" | binding=%s", strings.TrimSpace(match.Pairing.Binding))
			}
			if len(match.Pairing.Defaults) > 0 {
				line += fmt.Sprintf(" | defaults=%s", formatWorkflowDefaults(match.Pairing.Defaults))
			}
		}
		if strings.TrimSpace(match.Reason) != "" {
			line += " [" + strings.TrimSpace(match.Reason) + "]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func appendWorkflowPlanSummary(summary string, matches []plugin.AppWorkflowMatch) string {
	summary = strings.TrimSpace(summary)
	if len(matches) == 0 {
		return summary
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		name := match.Workflow.ToolName
		if match.Pairing != nil && strings.TrimSpace(match.Pairing.Name) != "" {
			name += "@" + strings.TrimSpace(match.Pairing.Name)
		}
		names = append(names, name)
	}
	note := "Suggested workflows: " + strings.Join(names, ", ")
	if summary == "" {
		return note
	}
	if strings.Contains(summary, note) {
		return summary
	}
	return summary + " " + note + "."
}

func formatWorkflowDefaults(items map[string]any) string {
	if len(items) == 0 {
		return ""
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(items))
	for _, key := range keys {
		value := items[key]
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ", ")
}
