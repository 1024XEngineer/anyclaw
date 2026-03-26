package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/prompt"
)

type Session struct {
	ID                string            `json:"id"`
	Title             string            `json:"title"`
	Agent             string            `json:"agent,omitempty"`
	Participants      []string          `json:"participants,omitempty"`
	Org               string            `json:"org,omitempty"`
	Project           string            `json:"project,omitempty"`
	Workspace         string            `json:"workspace,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	MessageCount      int               `json:"message_count"`
	LastUserText      string            `json:"last_user_text,omitempty"`
	History           []prompt.Message  `json:"history"`
	Messages          []SessionMessage  `json:"messages,omitempty"`
	SessionMode       string            `json:"session_mode,omitempty"`
	QueueMode         string            `json:"queue_mode,omitempty"`
	ReplyBack         bool              `json:"reply_back,omitempty"`
	SourceChannel     string            `json:"source_channel,omitempty"`
	SourceID          string            `json:"source_id,omitempty"`
	UserID            string            `json:"user_id,omitempty"`
	UserName          string            `json:"user_name,omitempty"`
	ReplyTarget       string            `json:"reply_target,omitempty"`
	ThreadID          string            `json:"thread_id,omitempty"`
	TransportMeta     map[string]string `json:"transport_meta,omitempty"`
	ParentSessionID   string            `json:"parent_session_id,omitempty"`
	GroupKey          string            `json:"group_key,omitempty"`
	IsGroup           bool              `json:"is_group,omitempty"`
	Presence          string            `json:"presence,omitempty"`
	Typing            bool              `json:"typing,omitempty"`
	QueueDepth        int               `json:"queue_depth,omitempty"`
	LastActiveAt      time.Time         `json:"last_active_at,omitempty"`
	LastAssistantText string            `json:"last_assistant_text,omitempty"`
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
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Input         string    `json:"input"`
	Status        string    `json:"status"`
	Assistant     string    `json:"assistant,omitempty"`
	Org           string    `json:"org,omitempty"`
	Project       string    `json:"project,omitempty"`
	Workspace     string    `json:"workspace,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	PlanSummary   string    `json:"plan_summary,omitempty"`
	Result        string    `json:"result,omitempty"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	StartedAt     string    `json:"started_at,omitempty"`
	CompletedAt   string    `json:"completed_at,omitempty"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
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

type Store struct {
	mu         sync.RWMutex
	path       string
	sessions   map[string]*Session
	events     []*Event
	tools      []*ToolActivityRecord
	tasks      []*Task
	taskSteps  []*TaskStep
	approvals  []*Approval
	audit      []*AuditEvent
	orgs       []*Org
	projects   []*Project
	workspaces []*Workspace
	jobs       []*Job
}

func NewStore(baseDir string) (*Store, error) {
	stateDir := filepath.Join(baseDir, "gateway")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	store := &Store{
		path:       filepath.Join(stateDir, "state.json"),
		sessions:   make(map[string]*Session),
		events:     []*Event{},
		tools:      []*ToolActivityRecord{},
		tasks:      []*Task{},
		taskSteps:  []*TaskStep{},
		approvals:  []*Approval{},
		audit:      []*AuditEvent{},
		orgs:       []*Org{},
		projects:   []*Project{},
		workspaces: []*Workspace{},
		jobs:       []*Job{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	for _, session := range state.Sessions {
		copied := cloneSession(session)
		s.sessions[copied.ID] = copied
	}
	s.events = cloneEvents(state.Events)
	s.tools = cloneToolActivities(state.Tools)
	s.tasks = cloneTasks(state.Tasks)
	s.taskSteps = cloneTaskSteps(state.TaskSteps)
	s.approvals = cloneApprovals(state.Approvals)
	s.audit = cloneAuditEvents(state.Audit)
	s.orgs = cloneOrgs(state.Orgs)
	s.projects = cloneProjects(state.Projects)
	s.workspaces = cloneWorkspaces(state.Workspaces)
	s.jobs = cloneJobs(state.Jobs)
	return nil
}

func (s *Store) SaveSession(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = cloneSession(session)
	return s.saveLocked()
}

func (s *Store) GetSession(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	return cloneSession(session), true
}

func (s *Store) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		list = append(list, cloneSession(session))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
	return list
}

func (s *Store) AppendEvent(event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneEvent(event))
	if len(s.events) > 200 {
		s.events = s.events[len(s.events)-200:]
	}
	return s.saveLocked()
}

func (s *Store) AppendToolActivity(activity *ToolActivityRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, cloneToolActivity(activity))
	if len(s.tools) > 500 {
		s.tools = s.tools[len(s.tools)-500:]
	}
	return s.saveLocked()
}

func (s *Store) AppendTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, cloneTask(task))
	return s.saveLocked()
}

func (s *Store) UpdateTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.tasks {
		if existing.ID == task.ID {
			s.tasks[i] = cloneTask(task)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("task not found: %s", task.ID)
}

func (s *Store) GetTask(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, task := range s.tasks {
		if task.ID == id {
			return cloneTask(task), true
		}
	}
	return nil, false
}

func (s *Store) ListTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := cloneTasks(s.tasks)
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastUpdatedAt.After(items[j].LastUpdatedAt)
	})
	return items
}

func (s *Store) AppendTaskStep(step *TaskStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskSteps = append(s.taskSteps, cloneTaskStep(step))
	return s.saveLocked()
}

func (s *Store) ReplaceTaskSteps(taskID string, steps []*TaskStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]*TaskStep, 0, len(s.taskSteps)+len(steps))
	for _, existing := range s.taskSteps {
		if existing.TaskID == taskID {
			continue
		}
		filtered = append(filtered, cloneTaskStep(existing))
	}
	for _, step := range steps {
		filtered = append(filtered, cloneTaskStep(step))
	}
	s.taskSteps = filtered
	return s.saveLocked()
}

func (s *Store) UpdateTaskStep(step *TaskStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.taskSteps {
		if existing.ID == step.ID {
			s.taskSteps[i] = cloneTaskStep(step)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("task step not found: %s", step.ID)
}

func (s *Store) ListTaskSteps(taskID string) []*TaskStep {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*TaskStep, 0, len(s.taskSteps))
	for _, step := range s.taskSteps {
		if taskID != "" && step.TaskID != taskID {
			continue
		}
		items = append(items, cloneTaskStep(step))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TaskID == items[j].TaskID {
			return items[i].Index < items[j].Index
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) AppendApproval(approval *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvals = append(s.approvals, cloneApproval(approval))
	return s.saveLocked()
}

func (s *Store) UpdateApproval(approval *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.approvals {
		if existing.ID == approval.ID {
			s.approvals[i] = cloneApproval(approval)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("approval not found: %s", approval.ID)
}

func (s *Store) GetApproval(id string) (*Approval, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, approval := range s.approvals {
		if approval.ID == id {
			return cloneApproval(approval), true
		}
	}
	return nil, false
}

func (s *Store) ListApprovals(status string) []*Approval {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*Approval, 0, len(s.approvals))
	for _, approval := range s.approvals {
		if status != "" && !strings.EqualFold(approval.Status, status) {
			continue
		}
		items = append(items, cloneApproval(approval))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].RequestedAt.After(items[j].RequestedAt)
	})
	return items
}

func (s *Store) ListTaskApprovals(taskID string) []*Approval {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*Approval, 0, len(s.approvals))
	for _, approval := range s.approvals {
		if taskID != "" && approval.TaskID != taskID {
			continue
		}
		items = append(items, cloneApproval(approval))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].RequestedAt.After(items[j].RequestedAt)
	})
	return items
}

func (s *Store) ListToolActivities(limit int, sessionID string) []*ToolActivityRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*ToolActivityRecord, 0, len(s.tools))
	for _, item := range s.tools {
		if sessionID != "" && item.SessionID != sessionID {
			continue
		}
		items = append(items, cloneToolActivity(item))
	}
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func (s *Store) ListEvents(limit int) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := cloneEvents(s.events)
	if limit > 0 && len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events
}

func (s *Store) AppendAudit(event *AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, cloneAuditEvent(event))
	if len(s.audit) > 500 {
		s.audit = s.audit[len(s.audit)-500:]
	}
	return s.saveLocked()
}

func (s *Store) ListAudit(limit int) []*AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := cloneAuditEvents(s.audit)
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func (s *Store) ListOrgs() []*Org {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneOrgs(s.orgs)
}

func (s *Store) ListProjects() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProjects(s.projects)
}

func (s *Store) ListWorkspaces() []*Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneWorkspaces(s.workspaces)
}

func (s *Store) AppendJob(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, cloneJob(job))
	if len(s.jobs) > 200 {
		s.jobs = s.jobs[len(s.jobs)-200:]
	}
	return s.saveLocked()
}

func (s *Store) UpdateJob(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.jobs {
		if existing.ID == job.ID {
			s.jobs[i] = cloneJob(job)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("job not found: %s", job.ID)
}

func (s *Store) ListJobs(limit int) []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := cloneJobs(s.jobs)
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func (s *Store) GetJob(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, job := range s.jobs {
		if job.ID == id {
			return cloneJob(job), true
		}
	}
	return nil, false
}

func (s *Store) UpsertOrg(org *Org) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.orgs {
		if existing.ID != org.ID && existing.Name == org.Name {
			return fmt.Errorf("org name already exists: %s", org.Name)
		}
	}
	replaced := false
	for i, existing := range s.orgs {
		if existing.ID == org.ID {
			s.orgs[i] = cloneOrg(org)
			replaced = true
			break
		}
	}
	if !replaced {
		s.orgs = append(s.orgs, cloneOrg(org))
	}
	return s.saveLocked()
}

func (s *Store) UpsertProject(project *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	orgExists := false
	for _, org := range s.orgs {
		if org.ID == project.OrgID {
			orgExists = true
			break
		}
	}
	if !orgExists {
		return fmt.Errorf("org not found for project: %s", project.OrgID)
	}
	for _, existing := range s.projects {
		if existing.ID != project.ID && existing.OrgID == project.OrgID && existing.Name == project.Name {
			return fmt.Errorf("project name already exists in org: %s", project.Name)
		}
	}
	replaced := false
	for i, existing := range s.projects {
		if existing.ID == project.ID {
			s.projects[i] = cloneProject(project)
			replaced = true
			break
		}
	}
	if !replaced {
		s.projects = append(s.projects, cloneProject(project))
	}
	return s.saveLocked()
}

func (s *Store) UpsertWorkspace(workspace *Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	projectExists := false
	for _, project := range s.projects {
		if project.ID == workspace.ProjectID {
			projectExists = true
			break
		}
	}
	if !projectExists {
		return fmt.Errorf("project not found for workspace: %s", workspace.ProjectID)
	}
	for _, existing := range s.workspaces {
		if existing.ID != workspace.ID && existing.ProjectID == workspace.ProjectID && existing.Name == workspace.Name {
			return fmt.Errorf("workspace name already exists in project: %s", workspace.Name)
		}
		if existing.ID != workspace.ID && existing.Path == workspace.Path {
			return fmt.Errorf("workspace path already exists: %s", workspace.Path)
		}
	}
	replaced := false
	for i, existing := range s.workspaces {
		if existing.ID == workspace.ID {
			s.workspaces[i] = cloneWorkspace(workspace)
			replaced = true
			break
		}
	}
	if !replaced {
		s.workspaces = append(s.workspaces, cloneWorkspace(workspace))
	}
	return s.saveLocked()
}

func (s *Store) GetOrg(id string) (*Org, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, org := range s.orgs {
		if org.ID == id {
			return cloneOrg(org), true
		}
	}
	return nil, false
}

func (s *Store) GetProject(id string) (*Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, project := range s.projects {
		if project.ID == id {
			return cloneProject(project), true
		}
	}
	return nil, false
}

func (s *Store) GetWorkspace(id string) (*Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, workspace := range s.workspaces {
		if workspace.ID == id {
			return cloneWorkspace(workspace), true
		}
	}
	return nil, false
}

func (s *Store) DeleteOrg(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, project := range s.projects {
		if project.OrgID == id {
			return fmt.Errorf("cannot delete org %s: dependent project %s exists", id, project.ID)
		}
	}
	for i, org := range s.orgs {
		if org.ID == id {
			s.orgs = append(s.orgs[:i], s.orgs[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("org not found: %s", id)
}

func (s *Store) DeleteProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, workspace := range s.workspaces {
		if workspace.ProjectID == id {
			return fmt.Errorf("cannot delete project %s: dependent workspace %s exists", id, workspace.ID)
		}
	}
	for i, project := range s.projects {
		if project.ID == id {
			s.projects = append(s.projects[:i], s.projects[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("project not found: %s", id)
}

func (s *Store) DeleteWorkspace(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		if session.Workspace == id {
			return fmt.Errorf("cannot delete workspace %s: dependent session %s exists", id, session.ID)
		}
	}
	for i, workspace := range s.workspaces {
		if workspace.ID == id {
			s.workspaces = append(s.workspaces[:i], s.workspaces[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("workspace not found: %s", id)
}

func (s *Store) RebindSessionsForProject(projectID string, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, session := range s.sessions {
		if session.Project == projectID {
			session.Org = orgID
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) RebindSessionsForWorkspace(workspaceID string, projectID string, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, session := range s.sessions {
		if session.Workspace == workspaceID {
			session.Project = projectID
			session.Org = orgID
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *Store) RebindWorkspaceID(oldID string, newID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(oldID) == "" || strings.TrimSpace(newID) == "" {
		return fmt.Errorf("workspace IDs must not be empty")
	}
	if oldID == newID {
		return nil
	}
	for _, workspace := range s.workspaces {
		if workspace.ID == newID {
			return fmt.Errorf("workspace already exists: %s", newID)
		}
	}
	changed := false
	for _, workspace := range s.workspaces {
		if workspace.ID == oldID {
			workspace.ID = newID
			changed = true
			break
		}
	}
	if !changed {
		return fmt.Errorf("workspace not found: %s", oldID)
	}
	for _, session := range s.sessions {
		if session.Workspace == oldID {
			session.Workspace = newID
		}
	}
	for _, task := range s.tasks {
		if task.Workspace == oldID {
			task.Workspace = newID
		}
	}
	for _, tool := range s.tools {
		if tool.Workspace == oldID {
			tool.Workspace = newID
		}
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, cloneSession(session))
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	state := persistedState{
		Sessions:   sessions,
		Tasks:      cloneTasks(s.tasks),
		TaskSteps:  cloneTaskSteps(s.taskSteps),
		Approvals:  cloneApprovals(s.approvals),
		Events:     cloneEvents(s.events),
		Tools:      cloneToolActivities(s.tools),
		Audit:      cloneAuditEvents(s.audit),
		Orgs:       cloneOrgs(s.orgs),
		Projects:   cloneProjects(s.projects),
		Workspaces: cloneWorkspaces(s.workspaces),
		Jobs:       cloneJobs(s.jobs),
		Updated:    time.Now().UTC(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

type SessionManager struct {
	mu      sync.Mutex
	store   *Store
	agent   SessionAgent
	nextID  func() string
	nowFunc func() time.Time
}

type SessionAgent interface {
	Run(ctx context.Context, userInput string) (string, error)
	GetHistory() []prompt.Message
	SetHistory(history []prompt.Message)
}

func NewSessionManager(store *Store, agent SessionAgent) *SessionManager {
	return &SessionManager{
		store: store,
		agent: agent,
		nextID: func() string {
			return uniqueID("sess")
		},
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func (m *SessionManager) Create(title string, agentName string, org string, project string, workspace string) (*Session, error) {
	return m.CreateWithOptions(SessionCreateOptions{Title: title, AgentName: agentName, Org: org, Project: project, Workspace: workspace})
}

type SessionCreateOptions struct {
	Title           string
	AgentName       string
	Participants    []string
	Org             string
	Project         string
	Workspace       string
	SessionMode     string
	QueueMode       string
	ReplyBack       bool
	SourceChannel   string
	SourceID        string
	UserID          string
	UserName        string
	ReplyTarget     string
	ThreadID        string
	TransportMeta   map[string]string
	ParentSessionID string
	GroupKey        string
	IsGroup         bool
}

func (m *SessionManager) CreateWithOptions(opts SessionCreateOptions) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.nowFunc()
	participants := normalizeParticipants(opts.AgentName, opts.Participants)
	primaryAgent := opts.AgentName
	if primaryAgent == "" && len(participants) > 0 {
		primaryAgent = participants[0]
	}
	session := &Session{
		ID:              m.nextID(),
		Title:           opts.Title,
		Agent:           primaryAgent,
		Participants:    participants,
		Org:             opts.Org,
		Project:         opts.Project,
		Workspace:       opts.Workspace,
		CreatedAt:       now,
		UpdatedAt:       now,
		History:         []prompt.Message{},
		Messages:        []SessionMessage{},
		SessionMode:     defaultSessionMode(opts.SessionMode),
		QueueMode:       defaultQueueMode(opts.QueueMode),
		ReplyBack:       opts.ReplyBack,
		SourceChannel:   opts.SourceChannel,
		SourceID:        opts.SourceID,
		UserID:          opts.UserID,
		UserName:        opts.UserName,
		ReplyTarget:     opts.ReplyTarget,
		ThreadID:        opts.ThreadID,
		TransportMeta:   cloneStringMap(opts.TransportMeta),
		ParentSessionID: opts.ParentSessionID,
		GroupKey:        opts.GroupKey,
		IsGroup:         opts.IsGroup,
		Presence:        "idle",
		Typing:          false,
		LastActiveAt:    now,
	}
	if session.Title == "" {
		session.Title = "New session"
	}
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) List() []*Session {
	return m.store.ListSessions()
}

func (m *SessionManager) Get(id string) (*Session, bool) {
	return m.store.GetSession(id)
}

func (m *SessionManager) AddExchange(sessionID string, userText string, assistantText string) (*Session, error) {
	messages := []SessionMessage{
		{
			ID:        uniqueID("msg"),
			Role:      "user",
			Content:   userText,
			CreatedAt: m.nowFunc(),
		},
		{
			ID:        uniqueID("msg"),
			Role:      "assistant",
			Content:   assistantText,
			CreatedAt: m.nowFunc(),
		},
	}
	return m.AddMessages(sessionID, messages)
}

func (m *SessionManager) AddMessages(sessionID string, messages []SessionMessage) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	now := m.nowFunc()
	for _, message := range messages {
		normalized := normalizeSessionMessage(message, now)
		session.Messages = append(session.Messages, normalized)
	}
	session.History = buildPromptHistory(session)
	session.MessageCount = len(session.Messages)
	session.LastUserText = lastSessionMessageContent(session.Messages, "user")
	session.LastAssistantText = lastSessionMessageContent(session.Messages, "assistant")
	session.UpdatedAt = now
	session.LastActiveAt = now
	session.Presence = "idle"
	session.Typing = false
	if session.QueueDepth > 0 {
		session.QueueDepth--
	}
	if session.Title == "New session" && session.LastUserText != "" {
		session.Title = shortenTitle(session.LastUserText)
	}
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) SetUserMapping(sessionID string, userID string, userName string, replyTarget string, threadID string, transportMeta map[string]string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if strings.TrimSpace(userID) != "" {
		session.UserID = strings.TrimSpace(userID)
	}
	if strings.TrimSpace(userName) != "" {
		session.UserName = strings.TrimSpace(userName)
	}
	if strings.TrimSpace(replyTarget) != "" {
		session.ReplyTarget = strings.TrimSpace(replyTarget)
	}
	if strings.TrimSpace(threadID) != "" {
		session.ThreadID = strings.TrimSpace(threadID)
	}
	if len(transportMeta) > 0 {
		if session.TransportMeta == nil {
			session.TransportMeta = map[string]string{}
		}
		for k, v := range transportMeta {
			if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
				session.TransportMeta[k] = v
			}
		}
	}
	session.UpdatedAt = m.nowFunc()
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) MoveSession(sessionID string, org string, project string, workspace string, agent string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if strings.TrimSpace(org) != "" {
		session.Org = org
	}
	if strings.TrimSpace(project) != "" {
		session.Project = project
	}
	if strings.TrimSpace(workspace) != "" {
		session.Workspace = workspace
	}
	if strings.TrimSpace(agent) != "" {
		session.Agent = agent
	}
	session.UpdatedAt = m.nowFunc()
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) SetPresence(sessionID string, presence string, typing bool) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if strings.TrimSpace(presence) != "" {
		session.Presence = strings.TrimSpace(presence)
	}
	session.Typing = typing
	session.UpdatedAt = m.nowFunc()
	session.LastActiveAt = session.UpdatedAt
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) EnqueueTurn(sessionID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	session.QueueDepth++
	session.Presence = "queued"
	session.UpdatedAt = m.nowFunc()
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func defaultSessionMode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "main"
	}
	return value
}

func defaultQueueMode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "fifo"
	}
	return value
}

func shortenTitle(input string) string {
	trimmed := input
	if len(trimmed) > 48 {
		trimmed = trimmed[:48]
	}
	if trimmed == "" {
		return "New session"
	}
	return trimmed
}

func cloneSession(session *Session) *Session {
	if session == nil {
		return nil
	}
	clone := *session
	clone.TransportMeta = cloneStringMap(session.TransportMeta)
	clone.Participants = append([]string(nil), session.Participants...)
	clone.History = append([]prompt.Message(nil), session.History...)
	clone.Messages = cloneSessionMessages(session.Messages)
	if len(clone.Messages) == 0 && len(clone.History) > 0 {
		clone.Messages = legacyMessagesFromHistory(&clone)
	}
	clone.MessageCount = len(clone.Messages)
	if clone.MessageCount > 0 {
		clone.LastUserText = lastSessionMessageContent(clone.Messages, "user")
		clone.LastAssistantText = lastSessionMessageContent(clone.Messages, "assistant")
	}
	return &clone
}

func cloneSessionMessage(message SessionMessage) SessionMessage {
	clone := message
	if message.Meta != nil {
		clone.Meta = make(map[string]any, len(message.Meta))
		for k, v := range message.Meta {
			clone.Meta[k] = v
		}
	}
	return clone
}

func cloneSessionMessages(messages []SessionMessage) []SessionMessage {
	if len(messages) == 0 {
		return nil
	}
	items := make([]SessionMessage, 0, len(messages))
	for _, message := range messages {
		items = append(items, cloneSessionMessage(message))
	}
	return items
}

func normalizeParticipants(primary string, participants []string) []string {
	seen := make(map[string]bool)
	items := make([]string, 0, len(participants)+1)
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		items = append(items, name)
	}
	appendName(primary)
	for _, name := range participants {
		appendName(name)
	}
	return items
}

func normalizeSessionMessage(message SessionMessage, fallbackTime time.Time) SessionMessage {
	if strings.TrimSpace(message.ID) == "" {
		message.ID = uniqueID("msg")
	}
	message.Role = strings.TrimSpace(strings.ToLower(message.Role))
	if message.Role == "" {
		message.Role = "assistant"
	}
	message.Agent = strings.TrimSpace(message.Agent)
	message.Kind = strings.TrimSpace(message.Kind)
	if message.CreatedAt.IsZero() {
		message.CreatedAt = fallbackTime
	}
	if message.Meta != nil {
		meta := make(map[string]any, len(message.Meta))
		for k, v := range message.Meta {
			meta[k] = v
		}
		message.Meta = meta
	}
	return message
}

func buildPromptHistory(session *Session) []prompt.Message {
	messages := session.Messages
	if len(messages) == 0 {
		return append([]prompt.Message(nil), session.History...)
	}
	history := make([]prompt.Message, 0, len(messages))
	group := len(normalizeParticipants(session.Agent, session.Participants)) > 1
	for _, message := range messages {
		switch message.Role {
		case "user":
			history = append(history, prompt.Message{Role: "user", Content: message.Content})
		case "assistant":
			content := message.Content
			if group && message.Agent != "" {
				content = fmt.Sprintf("[%s] %s", message.Agent, content)
			}
			history = append(history, prompt.Message{Role: "assistant", Content: content})
		case "system":
			history = append(history, prompt.Message{Role: "assistant", Content: fmt.Sprintf("[system] %s", message.Content)})
		}
	}
	return history
}

func lastSessionMessageContent(messages []SessionMessage, role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(strings.ToLower(messages[i].Role)) == role {
			return messages[i].Content
		}
	}
	return ""
}

func legacyMessagesFromHistory(session *Session) []SessionMessage {
	items := make([]SessionMessage, 0, len(session.History))
	for _, message := range session.History {
		role := strings.TrimSpace(strings.ToLower(message.Role))
		sessionMessage := SessionMessage{
			ID:        uniqueID("msg"),
			Role:      role,
			Content:   message.Content,
			CreatedAt: session.UpdatedAt,
		}
		if role == "assistant" {
			sessionMessage.Agent = session.Agent
		}
		items = append(items, sessionMessage)
	}
	return items
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	clone := make(map[string]string, len(input))
	for k, v := range input {
		clone[k] = v
	}
	return clone
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	return &clone
}

func cloneTasks(tasks []*Task) []*Task {
	items := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, cloneTask(task))
	}
	return items
}

func cloneTaskStep(step *TaskStep) *TaskStep {
	if step == nil {
		return nil
	}
	clone := *step
	return &clone
}

func cloneTaskSteps(steps []*TaskStep) []*TaskStep {
	items := make([]*TaskStep, 0, len(steps))
	for _, step := range steps {
		items = append(items, cloneTaskStep(step))
	}
	return items
}

func cloneApproval(approval *Approval) *Approval {
	if approval == nil {
		return nil
	}
	clone := *approval
	if approval.Payload != nil {
		clone.Payload = make(map[string]any, len(approval.Payload))
		for k, v := range approval.Payload {
			clone.Payload[k] = v
		}
	}
	return &clone
}

func cloneApprovals(items []*Approval) []*Approval {
	result := make([]*Approval, 0, len(items))
	for _, item := range items {
		result = append(result, cloneApproval(item))
	}
	return result
}

func cloneEvent(event *Event) *Event {
	if event == nil {
		return nil
	}
	clone := *event
	if event.Payload != nil {
		clone.Payload = make(map[string]any, len(event.Payload))
		for k, v := range event.Payload {
			clone.Payload[k] = v
		}
	}
	return &clone
}

func cloneEvents(events []*Event) []*Event {
	result := make([]*Event, 0, len(events))
	for _, event := range events {
		result = append(result, cloneEvent(event))
	}
	return result
}

func cloneAuditEvent(event *AuditEvent) *AuditEvent {
	if event == nil {
		return nil
	}
	clone := *event
	if event.Meta != nil {
		clone.Meta = make(map[string]any, len(event.Meta))
		for k, v := range event.Meta {
			clone.Meta[k] = v
		}
	}
	return &clone
}

func cloneAuditEvents(events []*AuditEvent) []*AuditEvent {
	result := make([]*AuditEvent, 0, len(events))
	for _, event := range events {
		result = append(result, cloneAuditEvent(event))
	}
	return result
}

func cloneToolActivity(activity *ToolActivityRecord) *ToolActivityRecord {
	if activity == nil {
		return nil
	}
	clone := *activity
	if activity.Args != nil {
		clone.Args = make(map[string]any, len(activity.Args))
		for k, v := range activity.Args {
			clone.Args[k] = v
		}
	}
	return &clone
}

func cloneToolActivities(items []*ToolActivityRecord) []*ToolActivityRecord {
	result := make([]*ToolActivityRecord, 0, len(items))
	for _, item := range items {
		result = append(result, cloneToolActivity(item))
	}
	return result
}

func cloneOrg(org *Org) *Org {
	if org == nil {
		return nil
	}
	clone := *org
	return &clone
}

func cloneOrgs(items []*Org) []*Org {
	result := make([]*Org, 0, len(items))
	for _, item := range items {
		result = append(result, cloneOrg(item))
	}
	return result
}

func cloneProject(project *Project) *Project {
	if project == nil {
		return nil
	}
	clone := *project
	return &clone
}

func cloneProjects(items []*Project) []*Project {
	result := make([]*Project, 0, len(items))
	for _, item := range items {
		result = append(result, cloneProject(item))
	}
	return result
}

func cloneWorkspace(workspace *Workspace) *Workspace {
	if workspace == nil {
		return nil
	}
	clone := *workspace
	return &clone
}

func cloneWorkspaces(items []*Workspace) []*Workspace {
	result := make([]*Workspace, 0, len(items))
	for _, item := range items {
		result = append(result, cloneWorkspace(item))
	}
	return result
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	clone := *job
	if job.Payload != nil {
		clone.Payload = make(map[string]any, len(job.Payload))
		for k, v := range job.Payload {
			clone.Payload[k] = v
		}
	}
	if job.Details != nil {
		clone.Details = make(map[string]any, len(job.Details))
		for k, v := range job.Details {
			clone.Details[k] = v
		}
	}
	return &clone
}

func cloneJobs(items []*Job) []*Job {
	result := make([]*Job, 0, len(items))
	for _, item := range items {
		result = append(result, cloneJob(item))
	}
	return result
}
