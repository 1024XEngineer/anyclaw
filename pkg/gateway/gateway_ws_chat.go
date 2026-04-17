package gateway

import (
	"context"
	"errors"
	"fmt"

	taskrunner "github.com/anyclaw/anyclaw/pkg/runtime/taskrunner"
	"github.com/anyclaw/anyclaw/pkg/state"
)

func (s *Server) wsChatSend(ctx context.Context, user *AuthUser, params map[string]any) (map[string]any, error) {
	message := mapString(params, "message")
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}
	title := mapString(params, "title")
	sessionID := mapString(params, "session_id")
	assistantName, err := s.mainEntryPolicy().NormalizeRequestedAgent(mapString(params, "agent"), mapString(params, "assistant"))
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		orgID := mapString(params, "org")
		projectID := mapString(params, "project")
		workspaceID := mapString(params, "workspace")
		if workspaceID == "" {
			orgID, projectID, workspaceID = defaultResourceIDs(s.mainRuntime.WorkingDir)
		}
		org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
		if err != nil {
			return nil, err
		}
		if !HasHierarchyAccess(user, org.ID, project.ID, workspace.ID) {
			return nil, fmt.Errorf("forbidden")
		}
		session, err := s.sessions.CreateWithOptions(state.SessionCreateOptions{
			Title:       title,
			AgentName:   assistantName,
			Org:         org.ID,
			Project:     project.ID,
			Workspace:   workspace.ID,
			SessionMode: "main",
			QueueMode:   "fifo",
		})
		if err != nil {
			return nil, err
		}
		sessionID = session.ID
		s.appendEvent("session.created", session.ID, sessionCreatedEventPayload(session))
	}
	response, updatedSession, err := s.runSessionMessage(ctx, sessionID, title, message)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			s.appendAudit(user, "chat.send", sessionID, map[string]any{"message_length": len(message), "transport": "ws", "status": "waiting_approval"})
			return s.sessionApprovalResponse(sessionID), nil
		}
		return nil, err
	}
	s.appendAudit(user, "chat.send", updatedSession.ID, map[string]any{"message_length": len(message), "transport": "ws"})
	return map[string]any{
		"response": response,
		"session":  updatedSession,
	}, nil
}
