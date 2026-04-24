package sessionrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	appruntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type Manager struct {
	store     *state.Store
	sessions  *state.SessionManager
	runtimes  RuntimeProvider
	approvals ApprovalRequester
	events    EventRecorder
	nowFunc   func() time.Time
	nextID    func(prefix string) string
	execute   func(context.Context, *appruntime.MainRuntime, appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error)
	stream    func(context.Context, *appruntime.MainRuntime, appruntime.ExecutionRequest, func(string)) (*appruntime.ExecutionResult, error)
}

type RuntimeProvider interface {
	GetOrCreate(agentName string, org string, project string, workspaceID string) (*appruntime.MainRuntime, error)
}

type ApprovalRequester interface {
	RequestWithSignature(taskID string, sessionID string, stepIndex int, toolName string, action string, payload map[string]any, signaturePayload map[string]any) (*state.Approval, error)
}

type EventRecorder interface {
	AppendEvent(eventType string, sessionID string, payload map[string]any)
}

type RunOptions struct {
	Source              string
	Resume              bool
	Channel             string
	ApprovalResumeState *agent.ApprovalResumeState
}

type RunRequest struct {
	SessionID string
	Title     string
	Message   string
	Options   RunOptions
}

type RunResult struct {
	Response string
	Session  *state.Session
}

type ApprovalContext struct {
	Title   string
	Message string
	Source  string
}

type ChannelRunRequest struct {
	Source    string
	SessionID string
	Message   string
	QueueMode string
	Meta      map[string]string
	Streaming bool
	OnChunk   func(string)
}

var ErrTaskWaitingApproval = taskrunner.ErrTaskWaitingApproval

func NewManager(store *state.Store, sessions *state.SessionManager, runtimes RuntimeProvider, approvals ApprovalRequester, events EventRecorder) *Manager {
	return &Manager{
		store:     store,
		sessions:  sessions,
		runtimes:  runtimes,
		approvals: approvals,
		events:    events,
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nextID: func(prefix string) string {
			return state.UniqueID(prefix)
		},
		execute: func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
			return runtime.Execute(ctx, req)
		},
		stream: func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest, onChunk func(string)) (*appruntime.ExecutionResult, error) {
			return runtime.Stream(ctx, req, onChunk)
		},
	}
}

func (m *Manager) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session creation now requires registered org/project/workspace via request path")
	}
	source := firstNonEmpty(strings.TrimSpace(req.Options.Source), "api")

	if !req.Options.Resume {
		if _, err := m.sessions.EnqueueTurn(sessionID); err == nil {
			m.appendEvent("session.queue.updated", sessionID, map[string]any{"queue_mode": "fifo", "source": source})
		}
	}
	if _, err := m.sessions.SetPresence(sessionID, "typing", true); err == nil {
		m.appendEvent("session.typing", sessionID, map[string]any{"typing": true, "source": source})
	}

	eventName := "chat.started"
	if req.Options.Resume {
		eventName = "chat.resumed"
	}
	m.appendEvent(eventName, sessionID, map[string]any{"message": req.Message, "source": source})

	session, ok := m.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	agentName, orgID, projectID, workspaceID := state.SessionExecutionTarget(session)
	targetRuntime, err := m.runtimes.GetOrCreate(agentName, orgID, projectID, workspaceID)
	if err != nil {
		failedSession := m.failRun(sessionID, req.Message, source, req.Options.Resume, session, err)
		return &RunResult{Session: failedSession}, err
	}

	if result, handled, err := m.tryDirectDesktopOpen(ctx, session, targetRuntime, ApprovalContext{
		Title:   req.Title,
		Message: req.Message,
		Source:  source,
	}, req.Options.Resume); handled {
		if err != nil {
			if errors.Is(err, ErrTaskWaitingApproval) {
				m.updateSessionApprovalPresence(sessionID, "")
			} else {
				failedSession := m.failRun(sessionID, req.Message, source, req.Options.Resume, session, err)
				if result == nil {
					result = &RunResult{}
				}
				result.Session = failedSession
			}
			return result, err
		}
		return result, nil
	}

	execResult, err := targetRuntime.Execute(ctx, appruntime.ExecutionRequest{
		Input:                req.Message,
		History:              session.History,
		ReplaceHistory:       true,
		SessionID:            sessionID,
		Channel:              firstNonEmpty(strings.TrimSpace(req.Options.Channel), "api"),
		AgentApprovalHook:    m.ToolApprovalHook(session, targetRuntime.Config, ApprovalContext{Title: req.Title, Message: req.Message, Source: source}),
		ApprovalResumeState:  req.Options.ApprovalResumeState,
		ProtocolApprovalHook: m.ProtocolApprovalHook(session, targetRuntime.Config, ApprovalContext{Title: req.Title, Message: req.Message, Source: source}),
	})

	response := ""
	persistedSession := session
	if execResult != nil {
		response = execResult.Output
	}
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			var pauseErr *agent.ApprovalPauseError
			if errors.As(err, &pauseErr) {
				m.persistApprovalResumeState(session, pauseErr)
			}
			if pendingSession, persistErr := m.sessions.EnsurePendingUserMessage(sessionID, req.Message); persistErr == nil {
				persistedSession = pendingSession
			}
			if execResult != nil && len(execResult.ToolActivities) > 0 && persistedSession != nil {
				m.RecordToolActivities(persistedSession, execResult.ToolActivities)
			}
			m.updateSessionApprovalPresence(sessionID, "")
		} else {
			persistedSession = m.failRun(sessionID, req.Message, source, req.Options.Resume, persistedSession, err)
		}
		return &RunResult{Response: response, Session: persistedSession}, err
	}

	var updatedSession *state.Session
	if req.Options.Resume {
		updatedSession, err = m.sessions.AddAssistantMessage(sessionID, response)
	} else {
		updatedSession, err = m.sessions.AddExchange(sessionID, req.Message, response)
	}
	if err != nil {
		failedSession := m.failRun(sessionID, req.Message, source, req.Options.Resume, session, err)
		return &RunResult{Response: response, Session: failedSession}, err
	}
	if _, err := m.sessions.SetPresence(sessionID, "idle", false); err == nil {
		m.appendEvent("session.presence", sessionID, map[string]any{"presence": "idle", "source": source})
	}
	if execResult != nil {
		m.RecordToolActivities(updatedSession, execResult.ToolActivities)
	}
	m.appendEvent("chat.completed", sessionID, map[string]any{"message": req.Message, "response_length": len(response), "source": source})
	return &RunResult{Response: response, Session: updatedSession}, nil
}

func (m *Manager) RunChannel(ctx context.Context, req ChannelRunRequest) (*RunResult, error) {
	session, targetRuntime, err := m.beginChannelExecution(req)
	if err != nil {
		return nil, err
	}

	response := ""
	activities := []agent.ToolActivity(nil)
	execReq := appruntime.ExecutionRequest{
		Input:                req.Message,
		History:              session.History,
		ReplaceHistory:       true,
		SessionID:            req.SessionID,
		Channel:              req.Source,
		AgentApprovalHook:    m.ToolApprovalHook(session, targetRuntime.Config, ApprovalContext{Message: req.Message, Source: req.Source}),
		ProtocolApprovalHook: m.ProtocolApprovalHook(session, targetRuntime.Config, ApprovalContext{Message: req.Message, Source: req.Source}),
	}
	if req.Streaming {
		execResult, streamErr := m.stream(ctx, targetRuntime, execReq, func(chunk string) {
			if req.OnChunk != nil {
				req.OnChunk(chunk)
			}
		})
		if execResult != nil {
			response = execResult.Output
			activities = execResult.ToolActivities
		}
		if streamErr != nil {
			return m.handleChannelExecutionError(req, session, response, activities, streamErr)
		}
	} else {
		execResult, execErr := m.execute(ctx, targetRuntime, execReq)
		if execResult != nil {
			response = execResult.Output
			activities = execResult.ToolActivities
		}
		if execErr != nil {
			return m.handleChannelExecutionError(req, session, response, activities, execErr)
		}
	}

	updatedSession, err := m.finishChannelExecution(req, response, activities)
	if err != nil {
		return &RunResult{Response: response, Session: session}, err
	}
	return &RunResult{Response: response, Session: updatedSession}, nil
}

func (m *Manager) handleChannelExecutionError(req ChannelRunRequest, session *state.Session, response string, activities []agent.ToolActivity, runErr error) (*RunResult, error) {
	persistedSession := session
	if errors.Is(runErr, ErrTaskWaitingApproval) {
		var pauseErr *agent.ApprovalPauseError
		if errors.As(runErr, &pauseErr) {
			m.persistApprovalResumeState(session, pauseErr)
		}
		if pendingSession, persistErr := m.sessions.EnsurePendingUserMessage(req.SessionID, req.Message); persistErr == nil {
			persistedSession = pendingSession
		}
		if len(activities) > 0 && persistedSession != nil {
			m.RecordToolActivities(persistedSession, activities)
		}
		m.updateSessionApprovalPresence(req.SessionID, "")
		return &RunResult{Response: response, Session: persistedSession}, runErr
	}

	persistedSession = m.failRun(req.SessionID, req.Message, req.Source, false, persistedSession, runErr)
	return &RunResult{Response: response, Session: persistedSession}, runErr
}

func (m *Manager) ToolApprovalHook(session *state.Session, cfg *config.Config, meta ApprovalContext) agent.ToolApprovalHook {
	if m == nil || m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous || session == nil {
		return nil
	}
	return func(ctx context.Context, tc agent.ToolCall) error {
		return m.RequireToolApproval(session, meta, tc.Name, tc.Args)
	}
}

func (m *Manager) ProtocolApprovalHook(session *state.Session, cfg *config.Config, meta ApprovalContext) tools.ToolApprovalHook {
	if m == nil || m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous || session == nil {
		return nil
	}
	return func(ctx context.Context, call tools.ToolApprovalCall) error {
		return m.RequireToolApproval(session, meta, call.Name, call.Args)
	}
}

func (m *Manager) RequireToolApproval(session *state.Session, meta ApprovalContext, toolName string, args map[string]any) error {
	if m == nil || session == nil {
		return nil
	}
	if !taskrunner.RequiresToolApprovalName(toolName) {
		return nil
	}

	workspaceID := state.SessionExecutionWorkspace(session)
	signaturePayload := map[string]any{
		"tool_name":  toolName,
		"args":       cloneAnyMap(args),
		"session_id": session.ID,
		"workspace":  workspaceID,
	}
	payload := cloneAnyMap(signaturePayload)
	payload["message"] = strings.TrimSpace(meta.Message)
	payload["title"] = strings.TrimSpace(meta.Title)
	signature := approvalSignature(toolName, "tool_call", signaturePayload)
	for _, approval := range m.store.ListSessionApprovals(session.ID) {
		if !matchesSessionToolApproval(approval, signature, session.ID, workspaceID, toolName, args) {
			continue
		}
		switch approval.Status {
		case "approved":
			return nil
		case "rejected":
			return fmt.Errorf("tool call rejected: %s", toolName)
		case "pending":
			m.updateSessionApprovalPresence(session.ID, toolName)
			return ErrTaskWaitingApproval
		}
	}

	approval, err := m.approvals.RequestWithSignature("", session.ID, 0, toolName, "tool_call", payload, signaturePayload)
	if err != nil {
		return err
	}
	m.updateSessionApprovalPresence(session.ID, toolName)
	m.appendEvent("approval.requested", session.ID, map[string]any{
		"approval_id": approval.ID,
		"tool_name":   toolName,
		"action":      "tool_call",
		"status":      approval.Status,
		"source":      firstNonEmpty(strings.TrimSpace(meta.Source), "session"),
	})
	return ErrTaskWaitingApproval
}

func (m *Manager) ResumeApproved(ctx context.Context, approval *state.Approval) error {
	if m == nil || approval == nil {
		return nil
	}
	sessionID := strings.TrimSpace(approval.SessionID)
	if sessionID == "" {
		return nil
	}
	message, _ := approval.Payload["message"].(string)
	title, _ := approval.Payload["title"].(string)
	message = strings.TrimSpace(message)
	title = strings.TrimSpace(title)
	resumeState := decodeApprovalResumeState(approval.Payload["resume_state"])
	if message == "" {
		return nil
	}

	_, err := m.Run(ctx, RunRequest{
		SessionID: sessionID,
		Title:     title,
		Message:   message,
		Options: RunOptions{
			Source:              "approval_resume",
			Resume:              true,
			Channel:             "api",
			ApprovalResumeState: resumeState,
		},
	})
	return err
}

func (m *Manager) AppendToolActivity(sessionID string, activity state.ToolActivityRecord) {
	if m == nil || m.store == nil {
		return
	}
	activity.ID = m.nextID("tool")
	activity.SessionID = sessionID
	if activity.Timestamp.IsZero() {
		activity.Timestamp = m.nowFunc()
	}
	_ = m.store.AppendToolActivity(&activity)
	m.appendEvent("tool.activity", sessionID, map[string]any{
		"tool_name": activity.ToolName,
		"args":      activity.Args,
		"error":     activity.Error,
		"agent":     activity.Agent,
		"workspace": activity.Workspace,
	})
}

func (m *Manager) RecordToolActivities(session *state.Session, activities []agent.ToolActivity) {
	if m == nil || session == nil {
		return
	}
	agentName := state.SessionExecutionAgent(session)
	workspaceID := state.SessionExecutionWorkspace(session)
	for _, activity := range activities {
		m.AppendToolActivity(session.ID, state.ToolActivityRecord{
			ToolName:  activity.ToolName,
			Args:      activity.Args,
			Result:    activity.Result,
			Error:     activity.Error,
			Agent:     agentName,
			Workspace: workspaceID,
		})
	}
}

func (m *Manager) tryDirectDesktopOpen(ctx context.Context, session *state.Session, targetRuntime *appruntime.MainRuntime, meta ApprovalContext, resume bool) (*RunResult, bool, error) {
	targetURL, ok := directDesktopOpenTarget(meta.Message)
	if !ok || session == nil || targetRuntime == nil {
		return nil, false, nil
	}
	agentName := state.SessionExecutionAgent(session)
	workspaceID := state.SessionExecutionWorkspace(session)
	input := map[string]any{
		"target": targetURL,
		"kind":   "url",
	}
	if err := m.RequireToolApproval(session, meta, "desktop_open", input); err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			if pendingSession, persistErr := m.sessions.EnsurePendingUserMessage(session.ID, meta.Message); persistErr == nil {
				return &RunResult{Session: pendingSession}, true, err
			}
		}
		return &RunResult{Session: session}, true, err
	}

	result, err := targetRuntime.CallTool(ctx, "desktop_open", input)
	if err != nil {
		m.AppendToolActivity(session.ID, state.ToolActivityRecord{
			ToolName:  "desktop_open",
			Args:      cloneAnyMap(input),
			Error:     err.Error(),
			Agent:     agentName,
			Workspace: workspaceID,
		})
		return &RunResult{Session: session}, true, err
	}

	response := fmt.Sprintf("Opened %s in the desktop browser.", targetURL)
	var updatedSession *state.Session
	if resume {
		updatedSession, err = m.sessions.AddAssistantMessage(session.ID, response)
	} else {
		updatedSession, err = m.sessions.AddExchange(session.ID, meta.Message, response)
	}
	if err != nil {
		return &RunResult{Response: response, Session: session}, true, err
	}
	if _, err := m.sessions.SetPresence(session.ID, "idle", false); err == nil {
		m.appendEvent("session.presence", session.ID, map[string]any{"presence": "idle", "source": firstNonEmpty(strings.TrimSpace(meta.Source), "api")})
	}
	m.AppendToolActivity(session.ID, state.ToolActivityRecord{
		ToolName:  "desktop_open",
		Args:      cloneAnyMap(input),
		Result:    result,
		Agent:     agentName,
		Workspace: workspaceID,
	})
	m.appendEvent("chat.completed", session.ID, map[string]any{
		"message":         meta.Message,
		"response_length": len(response),
		"source":          firstNonEmpty(strings.TrimSpace(meta.Source), "api"),
	})
	return &RunResult{Response: response, Session: updatedSession}, true, nil
}

func (m *Manager) beginChannelExecution(req ChannelRunRequest) (*state.Session, *appruntime.MainRuntime, error) {
	if _, err := m.sessions.EnqueueTurn(req.SessionID); err == nil {
		m.appendEvent("session.queue.updated", req.SessionID, map[string]any{
			"queue_mode":   req.QueueMode,
			"source":       req.Source,
			"reply_target": req.Meta["reply_target"],
		})
	}

	if _, err := m.sessions.SetUserMapping(
		req.SessionID,
		req.Meta["user_id"],
		firstNonEmpty(req.Meta["username"], req.Meta["user_name"]),
		req.Meta["reply_target"],
		req.Meta["thread_id"],
		channelTransportMeta(req.Meta),
	); err == nil {
		m.appendEvent("session.user_mapped", req.SessionID, map[string]any{
			"source":       req.Source,
			"user_id":      req.Meta["user_id"],
			"user_name":    firstNonEmpty(req.Meta["username"], req.Meta["user_name"]),
			"reply_target": req.Meta["reply_target"],
		})
	}

	if _, err := m.sessions.SetPresence(req.SessionID, "typing", true); err == nil {
		m.appendEvent("session.typing", req.SessionID, map[string]any{
			"typing":  true,
			"source":  req.Source,
			"user_id": req.Meta["user_id"],
		})
	}

	startedPayload := channelMetaPayload(map[string]any{
		"message": req.Message,
		"source":  req.Source,
	}, req.Meta)
	if req.Streaming {
		startedPayload["streaming"] = true
	}
	m.appendEvent("chat.started", req.SessionID, startedPayload)

	session, ok := m.sessions.Get(req.SessionID)
	if !ok {
		return nil, nil, fmt.Errorf("session not found: %s", req.SessionID)
	}
	if m.runtimes == nil {
		return nil, nil, fmt.Errorf("runtime pool not initialized")
	}

	agentName, orgID, projectID, workspaceID := state.SessionExecutionTarget(session)
	targetRuntime, err := m.runtimes.GetOrCreate(agentName, orgID, projectID, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	return session, targetRuntime, nil
}

func (m *Manager) finishChannelExecution(req ChannelRunRequest, response string, activities []agent.ToolActivity) (*state.Session, error) {
	updatedSession, err := m.sessions.AddExchange(req.SessionID, req.Message, response)
	if err != nil {
		return nil, err
	}
	if updatedSession.ReplyBack {
		m.appendEvent("session.reply_back", req.SessionID, map[string]any{
			"enabled":      true,
			"source":       req.Source,
			"reply_target": req.Meta["reply_target"],
		})
	}
	if _, err := m.sessions.SetPresence(req.SessionID, "idle", false); err == nil {
		m.appendEvent("session.presence", req.SessionID, map[string]any{
			"presence": "idle",
			"source":   req.Source,
			"user_id":  req.Meta["user_id"],
		})
	}
	if len(activities) > 0 {
		m.RecordToolActivities(updatedSession, activities)
	}

	completedPayload := channelMetaPayload(map[string]any{
		"message":         req.Message,
		"response_length": len(response),
		"source":          req.Source,
	}, req.Meta)
	if req.Streaming {
		completedPayload["streaming"] = true
	}
	m.appendEvent("chat.completed", req.SessionID, completedPayload)
	return updatedSession, nil
}

func (m *Manager) updateSessionApprovalPresence(sessionID string, toolName string) {
	m.updateSessionPresence(sessionID, "waiting_approval", false)
	m.appendEvent("session.presence", sessionID, map[string]any{
		"presence":  "waiting_approval",
		"tool_name": strings.TrimSpace(toolName),
		"source":    "approval",
	})
}

func (m *Manager) updateSessionPresence(sessionID string, presence string, typing bool) {
	if m == nil || m.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_, _ = m.sessions.SetPresence(sessionID, presence, typing)
}

func (m *Manager) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if m == nil || m.events == nil {
		return
	}
	m.events.AppendEvent(eventType, sessionID, payload)
}

func (m *Manager) failRun(sessionID string, message string, source string, resume bool, session *state.Session, runErr error) *state.Session {
	persistedSession := session
	if m == nil || m.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return persistedSession
	}
	if !resume {
		if pendingSession, err := m.sessions.EnsurePendingUserMessage(sessionID, message); err == nil {
			persistedSession = pendingSession
		}
	}
	if failedSession, err := m.sessions.FailTurn(sessionID); err == nil {
		persistedSession = failedSession
	} else {
		m.updateSessionPresence(sessionID, "idle", false)
	}
	if runErr != nil {
		m.appendEvent("chat.failed", sessionID, map[string]any{
			"message": message,
			"error":   runErr.Error(),
			"source":  source,
		})
	}
	return persistedSession
}

func matchesSessionToolApproval(approval *state.Approval, signature string, sessionID string, workspace string, toolName string, args map[string]any) bool {
	if approval == nil {
		return false
	}
	if approval.ToolName != toolName || approval.Action != "tool_call" || approval.SessionID != sessionID {
		return false
	}
	if approval.Signature == signature {
		return true
	}
	payloadWorkspace, _ := approval.Payload["workspace"].(string)
	if strings.TrimSpace(payloadWorkspace) != strings.TrimSpace(workspace) {
		return false
	}
	payloadArgs, ok := approval.Payload["args"]
	if !ok {
		return false
	}
	expectedArgs, err := json.Marshal(cloneAnyMap(args))
	if err != nil {
		return false
	}
	actualArgs, err := json.Marshal(payloadArgs)
	if err != nil {
		return false
	}
	return string(actualArgs) == string(expectedArgs)
}

func (m *Manager) persistApprovalResumeState(session *state.Session, pauseErr *agent.ApprovalPauseError) {
	if m == nil || m.store == nil || session == nil || pauseErr == nil {
		return
	}
	pending := m.findPendingToolApproval(session, pauseErr.State.PendingTool.Name, pauseErr.State.PendingTool.Args)
	if pending == nil {
		return
	}
	if pending.Payload == nil {
		pending.Payload = map[string]any{}
	}
	pending.Payload["resume_state"] = encodeApprovalResumeState(pauseErr.State)
	_ = m.store.UpdateApproval(pending)
}

func (m *Manager) findPendingToolApproval(session *state.Session, toolName string, args map[string]any) *state.Approval {
	if m == nil || m.store == nil || session == nil {
		return nil
	}
	workspaceID := state.SessionExecutionWorkspace(session)
	signaturePayload := map[string]any{
		"tool_name":  toolName,
		"args":       cloneAnyMap(args),
		"session_id": session.ID,
		"workspace":  workspaceID,
	}
	signature := approvalSignature(toolName, "tool_call", signaturePayload)
	for _, approval := range m.store.ListSessionApprovals(session.ID) {
		if approval == nil || approval.Status != "pending" {
			continue
		}
		if matchesSessionToolApproval(approval, signature, session.ID, workspaceID, toolName, args) {
			return approval
		}
	}
	return nil
}

func encodeApprovalResumeState(state agent.ApprovalResumeState) map[string]any {
	encoded := map[string]any{}
	data, err := json.Marshal(state)
	if err != nil {
		return encoded
	}
	_ = json.Unmarshal(data, &encoded)
	return encoded
}

func decodeApprovalResumeState(raw any) *agent.ApprovalResumeState {
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var state agent.ApprovalResumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	if strings.TrimSpace(state.PendingTool.Name) == "" {
		return nil
	}
	return &state
}

func approvalSignature(toolName string, action string, payload map[string]any) string {
	encoded, _ := json.Marshal(payload)
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(toolName), strings.TrimSpace(action), string(encoded))
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for k, v := range input {
		result[k] = v
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func channelMetaPayload(base map[string]any, meta map[string]string) map[string]any {
	payload := make(map[string]any, len(base)+len(meta))
	for k, v := range base {
		payload[k] = v
	}
	for k, v := range meta {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			payload[k] = trimmed
		}
	}
	return payload
}

func channelTransportMeta(meta map[string]string) map[string]string {
	transportMeta := map[string]string{}
	for _, key := range []string{"channel_id", "chat_id", "guild_id", "attachment_count"} {
		if v := strings.TrimSpace(meta[key]); v != "" {
			transportMeta[key] = v
		}
	}
	return transportMeta
}

func directDesktopOpenTarget(message string) (string, bool) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return "", false
	}
	lower := strings.ToLower(msg)
	hasOpenIntent := strings.Contains(lower, "open") || strings.Contains(lower, "visit") || strings.Contains(msg, "打开") || strings.Contains(msg, "访问")
	if !hasOpenIntent {
		return "", false
	}
	for _, field := range strings.Fields(msg) {
		trimmed := strings.Trim(field, " \t\r\n,，。！？；:?)（）[]【】>\"'")
		if strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://") {
			return trimmed, true
		}
	}
	aliases := map[string]string{
		"抖音":     "https://www.douyin.com/",
		"douyin": "https://www.douyin.com/",
	}
	for alias, target := range aliases {
		if strings.Contains(lower, strings.ToLower(alias)) || strings.Contains(msg, alias) {
			return target, true
		}
	}
	return "", false
}
