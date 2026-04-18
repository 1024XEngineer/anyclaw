package state

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SessionManager struct {
	mu      sync.Mutex
	store   *Store
	agent   SessionAgent
	nextID  func() string
	nowFunc func() time.Time
}

type SessionAgent interface {
	Run(ctx context.Context, userInput string) (string, error)
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
	ConversationKey string
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
		ID:           m.nextID(),
		Title:        opts.Title,
		Agent:        primaryAgent,
		Participants: nil,
		Org:          opts.Org,
		Project:      opts.Project,
		Workspace:    opts.Workspace,
		ExecutionBinding: SessionExecutionBinding{
			Agent:     primaryAgent,
			Org:       opts.Org,
			Project:   opts.Project,
			Workspace: opts.Workspace,
		},
		CreatedAt:       now,
		UpdatedAt:       now,
		History:         []HistoryMessage{},
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
		ConversationKey: strings.TrimSpace(opts.ConversationKey),
		TransportMeta:   cloneStringMap(opts.TransportMeta),
		ParentSessionID: opts.ParentSessionID,
		GroupKey:        "",
		IsGroup:         false,
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

func (m *SessionManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.DeleteSession(id)
}

func (m *SessionManager) FindByConversationKey(conversationKey string) (*Session, bool) {
	return m.store.FindSessionByConversationKey(conversationKey)
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

func (m *SessionManager) EnsurePendingUserMessage(sessionID string, userText string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	now := m.nowFunc()
	trimmed := strings.TrimSpace(userText)
	lastMessage := lastSessionMessage(session.Messages)
	if trimmed != "" && !(session.QueueDepth > 0 && lastMessage != nil && lastMessage.Role == "user" && lastMessage.Content == userText) {
		session.Messages = append(session.Messages, normalizeSessionMessage(SessionMessage{
			ID:        uniqueID("msg"),
			Role:      "user",
			Content:   userText,
			CreatedAt: now,
		}, now))
		session.History = buildSessionHistory(session)
		session.MessageCount = len(session.Messages)
		session.LastUserText = lastSessionMessageContent(session.Messages, "user")
		session.LastAssistantText = lastSessionMessageContent(session.Messages, "assistant")
	}
	session.UpdatedAt = now
	session.LastActiveAt = now
	if session.Title == "New session" && session.LastUserText != "" {
		session.Title = shortenTitle(session.LastUserText)
	}
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) AddAssistantMessage(sessionID string, assistantText string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	now := m.nowFunc()
	session.Messages = append(session.Messages, normalizeSessionMessage(SessionMessage{
		ID:        uniqueID("msg"),
		Role:      "assistant",
		Content:   assistantText,
		CreatedAt: now,
	}, now))
	session.History = buildSessionHistory(session)
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
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
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
	session.History = buildSessionHistory(session)
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

func (m *SessionManager) BindConversationKey(sessionID string, conversationKey string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	session.ConversationKey = strings.TrimSpace(conversationKey)
	session.UpdatedAt = m.nowFunc()
	session.LastActiveAt = session.UpdatedAt
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
		session.ExecutionBinding.Org = org
	}
	if strings.TrimSpace(project) != "" {
		session.Project = project
		session.ExecutionBinding.Project = project
	}
	if strings.TrimSpace(workspace) != "" {
		session.Workspace = workspace
		session.ExecutionBinding.Workspace = workspace
	}
	if strings.TrimSpace(agent) != "" {
		session.Agent = agent
		session.ExecutionBinding.Agent = agent
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
	if session.QueueDepth > 0 && session.Presence == "idle" {
		session.QueueDepth = 0
	}
	session.QueueDepth++
	session.Presence = "queued"
	session.UpdatedAt = m.nowFunc()
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (m *SessionManager) FailTurn(sessionID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.store.GetSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	now := m.nowFunc()
	if session.QueueDepth > 0 {
		session.QueueDepth--
	}
	session.Presence = "idle"
	session.Typing = false
	session.UpdatedAt = now
	session.LastActiveAt = now
	if session.Title == "New session" && session.LastUserText != "" {
		session.Title = shortenTitle(session.LastUserText)
	}
	if err := m.store.SaveSession(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func defaultSessionMode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "main"
	}
	switch value {
	case "group", "group-shared", "channel-group":
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
	normalizeSessionExecutionBinding(&clone)
	clone.TransportMeta = cloneStringMap(session.TransportMeta)
	clone.Participants = nil
	clone.GroupKey = ""
	clone.IsGroup = false
	clone.History = append([]HistoryMessage(nil), session.History...)
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

func normalizeSessionExecutionBinding(session *Session) {
	if session == nil {
		return
	}
	if strings.TrimSpace(session.ExecutionBinding.Agent) == "" {
		session.ExecutionBinding.Agent = strings.TrimSpace(session.Agent)
	}
	if strings.TrimSpace(session.ExecutionBinding.Org) == "" {
		session.ExecutionBinding.Org = strings.TrimSpace(session.Org)
	}
	if strings.TrimSpace(session.ExecutionBinding.Project) == "" {
		session.ExecutionBinding.Project = strings.TrimSpace(session.Project)
	}
	if strings.TrimSpace(session.ExecutionBinding.Workspace) == "" {
		session.ExecutionBinding.Workspace = strings.TrimSpace(session.Workspace)
	}
	if strings.TrimSpace(session.Agent) == "" {
		session.Agent = strings.TrimSpace(session.ExecutionBinding.Agent)
	}
	if strings.TrimSpace(session.Org) == "" {
		session.Org = strings.TrimSpace(session.ExecutionBinding.Org)
	}
	if strings.TrimSpace(session.Project) == "" {
		session.Project = strings.TrimSpace(session.ExecutionBinding.Project)
	}
	if strings.TrimSpace(session.Workspace) == "" {
		session.Workspace = strings.TrimSpace(session.ExecutionBinding.Workspace)
	}
}

func sessionExecutionBindingValue(session *Session) SessionExecutionBinding {
	if session == nil {
		return SessionExecutionBinding{}
	}
	normalized := *session
	normalizeSessionExecutionBinding(&normalized)
	return normalized.ExecutionBinding
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

func lastSessionMessage(messages []SessionMessage) *SessionMessage {
	if len(messages) == 0 {
		return nil
	}
	message := messages[len(messages)-1]
	return &message
}

func buildSessionHistory(session *Session) []HistoryMessage {
	messages := session.Messages
	if len(messages) == 0 {
		return append([]HistoryMessage(nil), session.History...)
	}
	history := make([]HistoryMessage, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "user":
			history = append(history, HistoryMessage{Role: "user", Content: message.Content})
		case "assistant":
			history = append(history, HistoryMessage{Role: "assistant", Content: message.Content})
		case "system":
			history = append(history, HistoryMessage{Role: "assistant", Content: fmt.Sprintf("[system] %s", message.Content)})
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
	agentName := sessionExecutionAgent(session)
	for _, message := range session.History {
		role := strings.TrimSpace(strings.ToLower(message.Role))
		sessionMessage := SessionMessage{
			ID:        uniqueID("msg"),
			Role:      role,
			Content:   message.Content,
			CreatedAt: session.UpdatedAt,
		}
		if role == "assistant" {
			sessionMessage.Agent = agentName
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
	clone.ExecutionState = cloneTaskExecutionState(task.ExecutionState)
	clone.Evidence = cloneTaskEvidenceList(task.Evidence)
	clone.RecoveryPoint = cloneTaskRecoveryPoint(task.RecoveryPoint)
	clone.Artifacts = cloneTaskArtifactList(task.Artifacts)
	return &clone
}

func cloneTaskEvidence(evidence *TaskEvidence) *TaskEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	clone.Data = cloneAnyMap(evidence.Data)
	return &clone
}

func cloneTaskEvidenceList(items []*TaskEvidence) []*TaskEvidence {
	if len(items) == 0 {
		return nil
	}
	result := make([]*TaskEvidence, 0, len(items))
	for _, item := range items {
		result = append(result, cloneTaskEvidence(item))
	}
	return result
}

func cloneTaskRecoveryPoint(point *TaskRecoveryPoint) *TaskRecoveryPoint {
	if point == nil {
		return nil
	}
	clone := *point
	clone.Data = cloneAnyMap(point.Data)
	return &clone
}

func cloneTaskArtifact(artifact *TaskArtifact) *TaskArtifact {
	if artifact == nil {
		return nil
	}
	clone := *artifact
	clone.Meta = cloneAnyMap(artifact.Meta)
	return &clone
}

func cloneTaskArtifactList(items []*TaskArtifact) []*TaskArtifact {
	if len(items) == 0 {
		return nil
	}
	result := make([]*TaskArtifact, 0, len(items))
	for _, item := range items {
		result = append(result, cloneTaskArtifact(item))
	}
	return result
}

func cloneTaskExecutionState(state *TaskExecutionState) *TaskExecutionState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.DesktopPlan = CloneDesktopPlanExecutionState(state.DesktopPlan)
	return &cloned
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
