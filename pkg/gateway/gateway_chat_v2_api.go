package gateway

import (
	"net/http"
	"strings"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
)

func (s *Server) handleV2Chat(w http.ResponseWriter, r *http.Request) {
	if s.chatModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat not available"})
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	req, commandReq, err := s.surfaceService().DecodeHTTPV2Chat(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := gatewaycommands.ValidateV2Chat(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	dispatch, err := s.commandIntakeService().Dispatch(commandReq)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if dispatch.Kind != "ingress" || dispatch.Target != "ingress" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unexpected command dispatch"})
		return
	}

	resp, err := s.chatModule.Chat(r.Context(), req)
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleV2ChatSessions(w http.ResponseWriter, r *http.Request) {
	if s.chatModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat not available"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	sessions := s.chatModule.ListSessions()
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleV2ChatSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.chatModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat not available"})
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/v2/chat/sessions/")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		history, err := s.chatModule.GetSessionHistory(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, history)

	case http.MethodDelete:
		if err := s.chatModule.DeleteSession(sessionID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
