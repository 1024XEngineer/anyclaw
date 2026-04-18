package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

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
	baseDir = filepath.Clean(baseDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	store := &Store{
		path:       filepath.Join(baseDir, "state.json"),
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
	changed := s.pruneOrphanedApprovalsLocked()
	if s.repairPendingSessionMessagesLocked() {
		changed = true
	}
	if changed {
		return s.saveLocked()
	}
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

func (s *Store) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	delete(s.sessions, id)
	s.deleteSessionApprovalsLocked(id)
	return s.saveLocked()
}

func (s *Store) FindSessionByConversationKey(conversationKey string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversationKey = strings.TrimSpace(conversationKey)
	if conversationKey == "" {
		return nil, false
	}

	var matched *Session
	for _, session := range s.sessions {
		if strings.TrimSpace(session.ConversationKey) != conversationKey {
			continue
		}
		if matched == nil || session.UpdatedAt.After(matched.UpdatedAt) {
			matched = session
		}
	}
	if matched == nil {
		return nil, false
	}
	return cloneSession(matched), true
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

func (s *Store) deleteSessionApprovalsLocked(sessionID string) bool {
	if strings.TrimSpace(sessionID) == "" || len(s.approvals) == 0 {
		return false
	}
	filtered := make([]*Approval, 0, len(s.approvals))
	changed := false
	for _, approval := range s.approvals {
		if approval == nil {
			changed = true
			continue
		}
		if strings.TrimSpace(approval.TaskID) == "" && approval.SessionID == sessionID {
			changed = true
			continue
		}
		filtered = append(filtered, approval)
	}
	if changed {
		s.approvals = filtered
	}
	return changed
}

func (s *Store) pruneOrphanedApprovalsLocked() bool {
	if len(s.approvals) == 0 {
		return false
	}
	filtered := make([]*Approval, 0, len(s.approvals))
	changed := false
	for _, approval := range s.approvals {
		if approval == nil {
			changed = true
			continue
		}
		taskID := strings.TrimSpace(approval.TaskID)
		if taskID != "" {
			if !s.hasTaskLocked(taskID) {
				changed = true
				continue
			}
			filtered = append(filtered, approval)
			continue
		}
		sessionID := strings.TrimSpace(approval.SessionID)
		if sessionID != "" {
			if _, ok := s.sessions[sessionID]; !ok {
				changed = true
				continue
			}
		}
		filtered = append(filtered, approval)
	}
	if changed {
		s.approvals = filtered
	}
	return changed
}

func (s *Store) hasTaskLocked(taskID string) bool {
	for _, task := range s.tasks {
		if task != nil && task.ID == taskID {
			return true
		}
	}
	return false
}

func (s *Store) repairPendingSessionMessagesLocked() bool {
	changed := false
	for _, approval := range s.approvals {
		if approval == nil || !strings.EqualFold(strings.TrimSpace(approval.Status), "pending") {
			continue
		}
		if strings.TrimSpace(approval.TaskID) != "" {
			continue
		}
		sessionID := strings.TrimSpace(approval.SessionID)
		if sessionID == "" {
			continue
		}
		session, ok := s.sessions[sessionID]
		if !ok || session == nil {
			continue
		}
		message, _ := approval.Payload["message"].(string)
		message = strings.TrimSpace(message)
		if message == "" || sessionHasTrailingUserMessage(session, message) {
			continue
		}
		timestamp := approval.RequestedAt
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		session.Messages = append(session.Messages, normalizeSessionMessage(SessionMessage{
			ID:        uniqueID("msg"),
			Role:      "user",
			Content:   message,
			CreatedAt: timestamp,
		}, timestamp))
		session.History = buildSessionHistory(session)
		session.MessageCount = len(session.Messages)
		session.LastUserText = lastSessionMessageContent(session.Messages, "user")
		session.LastAssistantText = lastSessionMessageContent(session.Messages, "assistant")
		if session.UpdatedAt.Before(timestamp) {
			session.UpdatedAt = timestamp
		}
		if session.LastActiveAt.Before(timestamp) {
			session.LastActiveAt = timestamp
		}
		if session.Title == "New session" && session.LastUserText != "" {
			session.Title = shortenTitle(session.LastUserText)
		}
		changed = true
	}
	return changed
}

func sessionHasTrailingUserMessage(session *Session, message string) bool {
	if session == nil {
		return false
	}
	lastMessage := lastSessionMessage(session.Messages)
	if lastMessage == nil || lastMessage.Role != "user" {
		return false
	}
	return strings.TrimSpace(lastMessage.Content) == strings.TrimSpace(message)
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

func (s *Store) ListSessionApprovals(sessionID string) []*Approval {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*Approval, 0, len(s.approvals))
	for _, approval := range s.approvals {
		if sessionID != "" && approval.SessionID != sessionID {
			continue
		}
		if strings.TrimSpace(approval.TaskID) != "" {
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
