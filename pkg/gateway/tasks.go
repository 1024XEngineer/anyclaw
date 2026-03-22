package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type TaskManager struct {
	store       *Store
	sessions    *SessionManager
	runtimePool *RuntimePool
	app         taskAppInfo
	planner     taskPlanner
	approvals   *approvalManager
	nextID      func(prefix string) string
	nowFunc     func() time.Time
}

type taskAppInfo struct {
	Name       string
	WorkingDir string
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

var ErrTaskWaitingApproval = errors.New("task waiting for approval")

func NewTaskManager(store *Store, sessions *SessionManager, runtimePool *RuntimePool, app taskAppInfo, planner taskPlanner, approvals *approvalManager) *TaskManager {
	return &TaskManager{
		store:       store,
		sessions:    sessions,
		runtimePool: runtimePool,
		app:         app,
		planner:     planner,
		approvals:   approvals,
		nextID: func(prefix string) string {
			return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
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

func (m *TaskManager) MarkRejected(taskID string, reason string) error {
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
	task.LastUpdatedAt = m.nowFunc()
	if err := m.store.UpdateTask(task); err != nil {
		return err
	}
	steps := m.store.ListTaskSteps(task.ID)
	for i, step := range steps {
		status := "skipped"
		if i == 1 {
			status = "failed"
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
	task.LastUpdatedAt = now
	if err := m.store.UpdateTask(task); err != nil {
		return nil, err
	}
	steps := m.store.ListTaskSteps(task.ID)
	if len(steps) == 0 {
		planSummary, planSteps := m.planTask(ctx, task.Input)
		task.PlanSummary = planSummary
		task.LastUpdatedAt = m.nowFunc()
		_ = m.store.UpdateTask(task)
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
	if len(steps) > 0 {
		_ = m.setStepStatus(task.ID, steps[0].Index, "completed", task.Input, "Task request normalized and accepted.", "")
	}
	if len(steps) > 1 {
		_ = m.setStepStatus(task.ID, steps[1].Index, "running", task.Input, "", "")
	}
	execMode := m.executionMode(task)
	if execMode.StrictSteps && len(steps) > 2 {
		_ = m.setStepStatus(task.ID, steps[2].Index, "running", "", "Preparing strict step execution.", "")
	}

	session, err := m.ensureSession(task)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
	}

	if _, err := m.sessions.EnqueueTurn(session.ID); err == nil {
		session, _ = m.sessions.SetPresence(session.ID, "typing", true)
	}
	app, err := m.runtimePool.GetOrCreate(task.Assistant, task.Org, task.Project, task.Workspace)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
	}
	if approvalErr := m.awaitApprovalsIfNeeded(task, session, app.Config); approvalErr != nil {
		if errors.Is(approvalErr, ErrTaskWaitingApproval) {
			return &TaskExecutionResult{Task: task, Session: session}, approvalErr
		}
		_ = m.failTask(task, approvalErr)
		return nil, approvalErr
	}
	app.Agent.SetHistory(session.History)
	execCtx := tools.WithBrowserSession(ctx, session.ID)
	execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: session.ID, Channel: "task"})
	execCtx = agent.WithToolApprovalHook(execCtx, m.toolApprovalHook(task, session, app.Config))
	response, err := app.Agent.Run(execCtx, task.Input)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
	}
	updatedSession, err := m.sessions.AddExchange(session.ID, task.Input, response)
	if err != nil {
		_ = m.failTask(task, err)
		return nil, err
	}
	_, _ = m.sessions.SetPresence(updatedSession.ID, "idle", false)

	task.Result = response
	task.Status = "completed"
	task.CompletedAt = m.nowFunc().Format(time.RFC3339)
	task.LastUpdatedAt = m.nowFunc()
	if err := m.store.UpdateTask(task); err != nil {
		return nil, err
	}
	if len(steps) > 1 {
		_ = m.setStepStatus(task.ID, steps[1].Index, "completed", task.Input, "Execution completed using the current runtime.", "")
	}
	if len(steps) > 2 {
		_ = m.setStepStatus(task.ID, steps[2].Index, "completed", "", response, "")
	}
	for i := 3; i < len(steps); i++ {
		_ = m.setStepStatus(task.ID, steps[i].Index, "completed", "", response, "")
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

func (m *TaskManager) awaitApprovalsIfNeeded(task *Task, session *Session, cfg *config.Config) error {
	if m.approvals == nil {
		return nil
	}
	if cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	if existing := m.findExecutionApproval(task.ID); existing != nil {
		switch existing.Status {
		case "approved":
			task.Status = "running"
			task.LastUpdatedAt = m.nowFunc()
			_ = m.store.UpdateTask(task)
			_ = m.setStepStatus(task.ID, 2, "running", task.Input, "Approval granted. Executing planned work.", "")
			return nil
		case "rejected":
			return fmt.Errorf("task execution rejected by approver")
		case "pending":
			task.Status = "waiting_approval"
			task.LastUpdatedAt = m.nowFunc()
			_ = m.store.UpdateTask(task)
			_ = m.setStepStatus(task.ID, 2, "waiting_approval", task.Input, "Awaiting approval before executing planned work.", "")
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
	_, err := m.approvals.Request(task.ID, session.ID, 2, "task_execution", "execute_task", payload)
	if err != nil {
		return err
	}
	task.Status = "waiting_approval"
	task.LastUpdatedAt = m.nowFunc()
	if err := m.store.UpdateTask(task); err != nil {
		return err
	}
	_ = m.setStepStatus(task.ID, 2, "waiting_approval", task.Input, "Awaiting approval before executing planned work.", "")
	return ErrTaskWaitingApproval
}

func (m *TaskManager) toolApprovalHook(task *Task, session *Session, cfg *config.Config) agent.ToolApprovalHook {
	if m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	return func(ctx context.Context, tc agent.ToolCall) error {
		if !requiresToolApproval(tc) {
			return nil
		}
		signature := approvalSignature(tc.Name, "tool_call", tc.Args)
		for _, approval := range m.store.ListTaskApprovals(task.ID) {
			if approval.Signature != signature || approval.ToolName != tc.Name || approval.Action != "tool_call" {
				continue
			}
			switch approval.Status {
			case "approved":
				return nil
			case "rejected":
				return fmt.Errorf("tool call rejected: %s", tc.Name)
			case "pending":
				task.Status = "waiting_approval"
				task.LastUpdatedAt = m.nowFunc()
				_ = m.store.UpdateTask(task)
				_ = m.setStepStatus(task.ID, 3, "waiting_approval", "", fmt.Sprintf("Awaiting approval for tool %s.", tc.Name), "")
				return ErrTaskWaitingApproval
			}
		}
		payload := map[string]any{
			"tool_name": tc.Name,
			"args":      tc.Args,
			"task_id":   task.ID,
			"workspace": task.Workspace,
		}
		_, err := m.approvals.Request(task.ID, session.ID, 3, tc.Name, "tool_call", payload)
		if err != nil {
			return err
		}
		task.Status = "waiting_approval"
		task.LastUpdatedAt = m.nowFunc()
		_ = m.store.UpdateTask(task)
		_ = m.setStepStatus(task.ID, 3, "waiting_approval", "", fmt.Sprintf("Awaiting approval for tool %s.", tc.Name), "")
		return ErrTaskWaitingApproval
	}
}

func requiresToolApproval(tc agent.ToolCall) bool {
	name := strings.TrimSpace(strings.ToLower(tc.Name))
	switch name {
	case "run_command", "write_file":
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
	task.LastUpdatedAt = m.nowFunc()
	if err := m.store.UpdateTask(task); err != nil {
		return nil, err
	}
	return session, nil
}

func (m *TaskManager) failTask(task *Task, err error) error {
	task.Status = "failed"
	task.Error = err.Error()
	task.CompletedAt = m.nowFunc().Format(time.RFC3339)
	task.LastUpdatedAt = m.nowFunc()
	steps := m.store.ListTaskSteps(task.ID)
	if len(steps) > 1 {
		_ = m.setStepStatus(task.ID, steps[1].Index, "failed", task.Input, "", err.Error())
	}
	for i := 2; i < len(steps); i++ {
		_ = m.setStepStatus(task.ID, steps[i].Index, "skipped", "", "", err.Error())
	}
	return m.store.UpdateTask(task)
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

func (m *TaskManager) planTask(ctx context.Context, input string) (string, []plannedStep) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultPlan(input)
	}
	if m.planner == nil {
		return defaultPlan(trimmed)
	}
	messages := []llm.Message{
		{Role: "system", Content: "You generate concise execution plans for local AI tasks. Return JSON only with fields summary and steps. Each step must contain title and kind. Use 3 to 6 steps. kinds can be analyze, inspect, execute, verify, summarize."},
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
	return payload.Summary, steps
}

func defaultPlan(input string) (string, []plannedStep) {
	trimmed := strings.TrimSpace(input)
	summary := "Analyze the request, inspect the workspace if needed, execute the task, and summarize the result."
	if trimmed != "" {
		summary = fmt.Sprintf("Analyze the request (%s), inspect the workspace if needed, execute the task, and summarize the result.", shortenTitle(trimmed))
	}
	return summary, []plannedStep{
		{Title: "Analyze the request", Kind: "analyze"},
		{Title: "Inspect relevant files or workspace context", Kind: "inspect"},
		{Title: "Execute the requested work", Kind: "execute"},
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
