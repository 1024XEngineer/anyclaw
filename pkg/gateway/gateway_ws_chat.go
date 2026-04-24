package gateway

import (
	"context"
	"errors"
	"fmt"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
)

func (s *Server) wsChatSend(ctx context.Context, user *AuthUser, params map[string]any) (map[string]any, error) {
	req, commandReq := s.surfaceService().DecodeWSChatSend(params)
	if err := gatewaycommands.ValidateChatSend(req); err != nil {
		return nil, err
	}
	dispatch, err := s.commandIntakeService().Dispatch(commandReq)
	if err != nil {
		return nil, err
	}
	assistantName, err := s.mainEntryPolicy().NormalizeRequestedAgent(req.Agent, req.Assistant)
	if err != nil {
		return nil, err
	}
	if req.SessionID == "" {
		orgID := req.OrgID
		projectID := req.ProjectID
		workspaceID := req.WorkspaceID
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
		req.OrgID = org.ID
		req.ProjectID = project.ID
		req.WorkspaceID = workspace.ID
	}
	response, updatedSession, err := s.runChatIngressMessage(ctx, "ws", req, assistantName)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			sessionID := req.SessionID
			if updatedSession != nil {
				sessionID = updatedSession.ID
			}
			s.appendAudit(user, "chat.send", sessionID, map[string]any{
				"message_length": len(req.Message),
				"transport":      "ws",
				"status":         "waiting_approval",
				"command_kind":   dispatch.Kind,
				"command_target": dispatch.Target,
			})
			return s.sessionApprovalResponse(sessionID), nil
		}
		return nil, err
	}
	s.appendAudit(user, "chat.send", updatedSession.ID, map[string]any{
		"message_length": len(req.Message),
		"transport":      "ws",
		"command_kind":   dispatch.Kind,
		"command_target": dispatch.Target,
	})
	return map[string]any{
		"response": response,
		"session":  updatedSession,
	}, nil
}
