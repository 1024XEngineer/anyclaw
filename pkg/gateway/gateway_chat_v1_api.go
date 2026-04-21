package gateway

import (
	"errors"
	"net/http"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	req, commandReq, err := s.surfaceService().DecodeHTTPChatSend(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if err := gatewaycommands.ValidateChatSend(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	dispatch, err := s.commandIntakeService().Dispatch(commandReq)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	agentName, err := s.mainEntryPolicy().NormalizeRequestedAgent(req.Agent, req.Assistant)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.SessionID == "" {
		orgID, projectID, workspaceID := req.OrgID, req.ProjectID, req.WorkspaceID
		org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !HasHierarchyAccess(UserFromContext(r.Context()), org.ID, project.ID, workspace.ID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_org": org.ID, "required_project": project.ID, "required_workspace": workspace.ID})
			return
		}
		req.OrgID = org.ID
		req.ProjectID = project.ID
		req.WorkspaceID = workspace.ID
	}

	response, updatedSession, err := s.runChatIngressMessage(r.Context(), "api", req, agentName)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			sessionID := req.SessionID
			if updatedSession != nil {
				sessionID = updatedSession.ID
			}
			s.appendAudit(UserFromContext(r.Context()), "chat.send", sessionID, map[string]any{
				"message_length": len(req.Message),
				"status":         "waiting_approval",
				"command_kind":   dispatch.Kind,
				"command_target": dispatch.Target,
			})
			writeJSON(w, http.StatusAccepted, s.sessionApprovalResponse(sessionID))
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "chat.send", updatedSession.ID, map[string]any{
		"message_length": len(req.Message),
		"command_kind":   dispatch.Kind,
		"command_target": dispatch.Target,
	})
	writeJSON(w, http.StatusOK, map[string]any{"response": response, "session": updatedSession})
}
