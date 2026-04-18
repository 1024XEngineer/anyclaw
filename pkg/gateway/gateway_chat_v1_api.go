package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
		Agent     string `json:"agent"`
		Assistant string `json:"assistant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	agentName, err := s.mainEntryPolicy().NormalizeRequestedAgent(req.Agent, req.Assistant)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		orgID, projectID, workspaceID := s.resolveResourceSelection(r)
		org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !HasHierarchyAccess(UserFromContext(r.Context()), org.ID, project.ID, workspace.ID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_org": org.ID, "required_project": project.ID, "required_workspace": workspace.ID})
			return
		}
		session, err := s.sessions.CreateWithOptions(state.SessionCreateOptions{
			Title:       req.Title,
			AgentName:   agentName,
			Org:         org.ID,
			Project:     project.ID,
			Workspace:   workspace.ID,
			SessionMode: "main",
			QueueMode:   "fifo",
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		req.SessionID = session.ID
		s.appendEvent("session.created", session.ID, sessionCreatedEventPayload(session))
	}

	response, updatedSession, err := s.runSessionMessage(r.Context(), req.SessionID, req.Title, req.Message)
	if err != nil {
		if errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
			s.appendAudit(UserFromContext(r.Context()), "chat.send", req.SessionID, map[string]any{"message_length": len(req.Message), "status": "waiting_approval"})
			writeJSON(w, http.StatusAccepted, s.sessionApprovalResponse(req.SessionID))
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "chat.send", updatedSession.ID, map[string]any{"message_length": len(req.Message)})
	writeJSON(w, http.StatusOK, map[string]any{"response": response, "session": updatedSession})
}
