package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	appruntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type Manager struct {
	store       *state.Store
	sessions    *state.SessionManager
	runtimes    RuntimeProvider
	mainRuntime MainRuntimeInfo
	planner     Planner
	approvals   ApprovalRequester
	nextID      func(prefix string) string
	nowFunc     func() time.Time
}

type MainRuntimeInfo struct {
	Name       string
	WorkingDir string
	ConfigPath string
}

type Planner interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
	Name() string
}

type RuntimeProvider interface {
	GetOrCreate(agentName string, org string, project string, workspaceID string) (*appruntime.MainRuntime, error)
}

type ApprovalRequester interface {
	Request(taskID string, sessionID string, stepIndex int, toolName string, action string, payload map[string]any) (*state.Approval, error)
}

type CreateOptions struct {
	Title     string
	Input     string
	Assistant string
	Org       string
	Project   string
	Workspace string
	SessionID string
}

type ExecutionResult struct {
	Task           *state.Task
	Session        *state.Session
	ToolActivities []agent.ToolActivity
}

type PlanStep struct {
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

type plannedStep = PlanStep

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

func NewManager(store *state.Store, sessions *state.SessionManager, runtimes RuntimeProvider, mainRuntime MainRuntimeInfo, planner Planner, approvals ApprovalRequester) *Manager {
	return &Manager{
		store:       store,
		sessions:    sessions,
		runtimes:    runtimes,
		mainRuntime: mainRuntime,
		planner:     planner,
		approvals:   approvals,
		nextID: func(prefix string) string {
			return state.UniqueID(prefix)
		},
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *Manager) Create(opts CreateOptions) (*state.Task, error) {
	now := m.nowFunc()
	planSummary, stepDefs := m.planTask(context.Background(), strings.TrimSpace(opts.Input))
	task := &state.Task{
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
		task.Title = state.ShortenTitle(task.Input)
	}
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
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
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
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
	steps := make([]*state.TaskStep, 0, len(stepDefs))
	for i, def := range stepDefs {
		step := &state.TaskStep{
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

func (m *Manager) List() []*state.Task {
	return m.store.ListTasks()
}

func (m *Manager) Get(id string) (*state.Task, bool) {
	return m.store.GetTask(id)
}

func (m *Manager) Steps(taskID string) []*state.TaskStep {
	return m.store.ListTaskSteps(taskID)
}

func (m *Manager) MarkRejected(taskID string, stepIndex int, reason string) error {
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
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
		Kind:      "approval_rejected",
		Summary:   "Task execution was rejected during approval.",
		Detail:    reason,
		StepIndex: stepIndex,
		Status:    task.Status,
		Source:    "approval",
	})
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
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

func (m *Manager) Execute(ctx context.Context, taskID string) (*ExecutionResult, error) {
	task, ok := m.store.GetTask(taskID)
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status == "completed" {
		return &ExecutionResult{Task: task}, nil
	}
	now := m.nowFunc()
	task.Status = "running"
	task.StartedAt = now.Format(time.RFC3339)
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
		Kind:      "execution_started",
		Summary:   "Task execution started.",
		Detail:    task.PlanSummary,
		StepIndex: 2,
		Status:    task.Status,
		Source:    "task_manager",
	})
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
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
		m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
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
		rebuilt := make([]*state.TaskStep, 0, len(planSteps))
		for i, def := range planSteps {
			rebuilt = append(rebuilt, &state.TaskStep{ID: m.nextID("taskstep"), TaskID: task.ID, Index: i + 1, Title: def.Title, Kind: def.Kind, Status: "pending", CreatedAt: now, UpdatedAt: now})
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
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
		Kind:      "session_ready",
		Summary:   "Task is bound to an execution session.",
		StepIndex: 2,
		Status:    task.Status,
		Source:    "task_manager",
		Data: map[string]any{
			"session_id": session.ID,
		},
	})
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
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
	targetRuntime, err := m.runtimes.GetOrCreate(task.Assistant, task.Org, task.Project, task.Workspace)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
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

	if approvalErr := m.awaitApprovalsIfNeeded(task, session, targetRuntime.Config, firstNonZero(stage.prepare, stage.execute, 2)); approvalErr != nil {
		if errors.Is(approvalErr, ErrTaskWaitingApproval) {
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return &ExecutionResult{Task: task, Session: session}, approvalErr
		}
		m.updateSessionPresence(session.ID, "idle", false)
		_ = m.failTask(task, approvalErr)
		return nil, approvalErr
	}
	if stage.prepare > 0 && stage.prepare != stage.execute {
		_ = m.setStepStatus(task.ID, stage.prepare, "completed", task.Input, "Runtime and workspace context are ready for execution.", "")
	}
	if stage.execute > 0 {
		_ = m.setStepStatus(task.ID, stage.execute, "running", "", executionStageOutput(task, nil), "")
	}
	req := appruntime.ExecutionRequest{
		Input:                task.Input,
		History:              session.History,
		ReplaceHistory:       true,
		SessionID:            session.ID,
		Channel:              "task",
		AgentApprovalHook:    m.toolApprovalHook(task, session, targetRuntime.Config),
		ProtocolApprovalHook: m.protocolApprovalHook(task, session, targetRuntime.Config),
	}
	if task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		req.DesktopPlanResumeState = task.ExecutionState.DesktopPlan
		m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
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
	req.DesktopPlanStateHook = m.desktopPlanStateHook(task)
	execResult, err := targetRuntime.Execute(ctx, req)
	if freshTask, ok := m.store.GetTask(task.ID); ok && freshTask != nil {
		task = freshTask
	}
	toolActivities := []agent.ToolActivity(nil)
	response := ""
	if execResult != nil {
		toolActivities = execResult.ToolActivities
		response = execResult.Output
	}
	m.recordTaskToolActivitiesNoSave(task, toolActivities)
	if len(toolActivities) > 0 {
		_ = m.persistTask(task)
	}
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return &ExecutionResult{Task: task, Session: session, ToolActivities: toolActivities}, err
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
		_ = m.setStepStatus(task.ID, stage.prepare, "completed", task.Input, "Runtime and workspace context are ready for execution.", "")
	}
	if stage.execute > 0 {
		_ = m.setStepStatus(task.ID, stage.execute, "completed", "", executionStageOutput(task, toolActivities), "")
	}
	verificationOutput, verificationObserved := verificationStageOutput(task, toolActivities)
	if stage.verify > 0 {
		_ = m.setStepStatus(task.ID, stage.verify, "completed", "", verificationOutput, "")
		m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
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
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
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
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
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

	return &ExecutionResult{Task: task, Session: updatedSession, ToolActivities: toolActivities}, nil
}

func (m *Manager) ensureSession(task *state.Task) (*state.Session, error) {
	if strings.TrimSpace(task.SessionID) != "" {
		session, ok := m.sessions.Get(task.SessionID)
		if ok {
			return session, nil
		}
	}
	session, err := m.sessions.CreateWithOptions(state.SessionCreateOptions{
		Title:     task.Title,
		AgentName: firstNonEmpty(task.Assistant, m.mainRuntime.Name),
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

func (m *Manager) failTask(task *state.Task, err error) error {
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
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
		Kind:      "task_failed",
		Summary:   "Task execution failed.",
		Detail:    err.Error(),
		StepIndex: failedIndex,
		Status:    task.Status,
		Source:    "task_manager",
		Data:      recoveryData,
	})
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
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

func (m *Manager) setStepStatus(taskID string, index int, status string, input string, output string, stepErr string) error {
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

func (m *Manager) executionStageIndexes(taskID string) taskStageIndexes {
	return locateTaskStageIndexes(m.store.ListTaskSteps(taskID))
}

func locateTaskStageIndexes(steps []*state.TaskStep) taskStageIndexes {
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

func firstTaskStepIndexByKinds(steps []*state.TaskStep, kinds ...string) int {
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
