package state

import (
	"time"
)

type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type DesktopPlanStepExecutionState struct {
	Index     int    `json:"index"`
	Tool      string `json:"tool"`
	Label     string `json:"label,omitempty"`
	HasVerify bool   `json:"has_verify,omitempty"`
	Verified  bool   `json:"verified,omitempty"`
	Status    string `json:"status,omitempty"`
	Attempts  int    `json:"attempts,omitempty"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type DesktopPlanExecutionState struct {
	ToolName          string                          `json:"tool_name,omitempty"`
	Plugin            string                          `json:"plugin,omitempty"`
	App               string                          `json:"app,omitempty"`
	Action            string                          `json:"action,omitempty"`
	Workflow          string                          `json:"workflow,omitempty"`
	Status            string                          `json:"status,omitempty"`
	Summary           string                          `json:"summary,omitempty"`
	Result            string                          `json:"result,omitempty"`
	TotalSteps        int                             `json:"total_steps,omitempty"`
	CurrentStep       int                             `json:"current_step,omitempty"`
	NextStep          int                             `json:"next_step,omitempty"`
	LastCompletedStep int                             `json:"last_completed_step,omitempty"`
	LastOutput        string                          `json:"last_output,omitempty"`
	LastError         string                          `json:"last_error,omitempty"`
	Resumed           bool                            `json:"resumed,omitempty"`
	Steps             []DesktopPlanStepExecutionState `json:"steps,omitempty"`
	UpdatedAt         string                          `json:"updated_at,omitempty"`
}

func CloneDesktopPlanExecutionState(state *DesktopPlanExecutionState) *DesktopPlanExecutionState {
	if state == nil {
		return nil
	}
	cloned := *state
	if len(state.Steps) > 0 {
		cloned.Steps = append([]DesktopPlanStepExecutionState(nil), state.Steps...)
	}
	return &cloned
}

type Session struct {
	ID                string                  `json:"id"`
	Title             string                  `json:"title"`
	Agent             string                  `json:"agent,omitempty"`
	Participants      []string                `json:"participants,omitempty"`
	Org               string                  `json:"org,omitempty"`
	Project           string                  `json:"project,omitempty"`
	Workspace         string                  `json:"workspace,omitempty"`
	ExecutionBinding  SessionExecutionBinding `json:"execution_binding,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
	MessageCount      int                     `json:"message_count"`
	LastUserText      string                  `json:"last_user_text,omitempty"`
	History           []HistoryMessage        `json:"history"`
	Messages          []SessionMessage        `json:"messages,omitempty"`
	SessionMode       string                  `json:"session_mode,omitempty"`
	QueueMode         string                  `json:"queue_mode,omitempty"`
	ReplyBack         bool                    `json:"reply_back,omitempty"`
	SourceChannel     string                  `json:"source_channel,omitempty"`
	SourceID          string                  `json:"source_id,omitempty"`
	UserID            string                  `json:"user_id,omitempty"`
	UserName          string                  `json:"user_name,omitempty"`
	ReplyTarget       string                  `json:"reply_target,omitempty"`
	ThreadID          string                  `json:"thread_id,omitempty"`
	ConversationKey   string                  `json:"conversation_key,omitempty"`
	TransportMeta     map[string]string       `json:"transport_meta,omitempty"`
	ParentSessionID   string                  `json:"parent_session_id,omitempty"`
	GroupKey          string                  `json:"group_key,omitempty"`
	IsGroup           bool                    `json:"is_group,omitempty"`
	Presence          string                  `json:"presence,omitempty"`
	Typing            bool                    `json:"typing,omitempty"`
	QueueDepth        int                     `json:"queue_depth,omitempty"`
	LastActiveAt      time.Time               `json:"last_active_at,omitempty"`
	LastAssistantText string                  `json:"last_assistant_text,omitempty"`
}

type SessionExecutionBinding struct {
	Agent     string `json:"agent,omitempty"`
	Org       string `json:"org,omitempty"`
	Project   string `json:"project,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

type SessionMessage struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Agent     string         `json:"agent,omitempty"`
	Content   string         `json:"content"`
	Kind      string         `json:"kind,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type persistedState struct {
	Sessions   []*Session            `json:"sessions"`
	Tasks      []*Task               `json:"tasks,omitempty"`
	TaskSteps  []*TaskStep           `json:"task_steps,omitempty"`
	Approvals  []*Approval           `json:"approvals,omitempty"`
	Events     []*Event              `json:"events"`
	Tools      []*ToolActivityRecord `json:"tools"`
	Audit      []*AuditEvent         `json:"audit"`
	Orgs       []*Org                `json:"orgs"`
	Projects   []*Project            `json:"projects"`
	Workspaces []*Workspace          `json:"workspaces"`
	Jobs       []*Job                `json:"jobs"`
	Updated    time.Time             `json:"updated"`
}

type AuditEvent struct {
	ID        string         `json:"id"`
	Actor     string         `json:"actor"`
	Role      string         `json:"role"`
	Action    string         `json:"action"`
	Target    string         `json:"target"`
	Timestamp time.Time      `json:"timestamp"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type ToolActivityRecord struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id,omitempty"`
	ToolName  string         `json:"tool_name"`
	Args      map[string]any `json:"args,omitempty"`
	Result    string         `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Agent     string         `json:"agent,omitempty"`
	Workspace string         `json:"workspace,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type Org struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
	Name  string `json:"name"`
}

type Workspace struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

type Job struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Status      string         `json:"status"`
	Summary     string         `json:"summary"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   string         `json:"started_at,omitempty"`
	CompletedAt string         `json:"completed_at,omitempty"`
	Error       string         `json:"error,omitempty"`
	RetryOf     string         `json:"retry_of,omitempty"`
	Cancellable bool           `json:"cancellable,omitempty"`
	Retriable   bool           `json:"retriable,omitempty"`
	Attempts    int            `json:"attempts,omitempty"`
	MaxAttempts int            `json:"max_attempts,omitempty"`
	NextRunAt   string         `json:"next_run_at,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

type Task struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Input          string              `json:"input"`
	Status         string              `json:"status"`
	Assistant      string              `json:"assistant,omitempty"`
	Org            string              `json:"org,omitempty"`
	Project        string              `json:"project,omitempty"`
	Workspace      string              `json:"workspace,omitempty"`
	SessionID      string              `json:"session_id,omitempty"`
	PlanSummary    string              `json:"plan_summary,omitempty"`
	ExecutionState *TaskExecutionState `json:"execution_state,omitempty"`
	Evidence       []*TaskEvidence     `json:"evidence,omitempty"`
	RecoveryPoint  *TaskRecoveryPoint  `json:"recovery_point,omitempty"`
	Artifacts      []*TaskArtifact     `json:"artifacts,omitempty"`
	Result         string              `json:"result,omitempty"`
	Error          string              `json:"error,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	StartedAt      string              `json:"started_at,omitempty"`
	CompletedAt    string              `json:"completed_at,omitempty"`
	LastUpdatedAt  time.Time           `json:"last_updated_at"`
}

type TaskEvidence struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Summary   string         `json:"summary"`
	Detail    string         `json:"detail,omitempty"`
	StepIndex int            `json:"step_index,omitempty"`
	Status    string         `json:"status,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Source    string         `json:"source,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type TaskRecoveryPoint struct {
	Kind      string         `json:"kind"`
	Summary   string         `json:"summary,omitempty"`
	StepIndex int            `json:"step_index,omitempty"`
	Status    string         `json:"status,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

type TaskArtifact struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Label       string         `json:"label,omitempty"`
	Path        string         `json:"path,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

type TaskExecutionState struct {
	DesktopPlan *DesktopPlanExecutionState `json:"desktop_plan,omitempty"`
}

type TaskStep struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Index     int       `json:"index"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Input     string    `json:"input,omitempty"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Approval struct {
	ID          string         `json:"id"`
	TaskID      string         `json:"task_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	StepIndex   int            `json:"step_index,omitempty"`
	ToolName    string         `json:"tool_name"`
	Action      string         `json:"action,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	Signature   string         `json:"signature"`
	Status      string         `json:"status"`
	RequestedAt time.Time      `json:"requested_at"`
	ResolvedAt  string         `json:"resolved_at,omitempty"`
	ResolvedBy  string         `json:"resolved_by,omitempty"`
	Comment     string         `json:"comment,omitempty"`
}
