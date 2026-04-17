package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/gateway/intake/chat"
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

	var req chat.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if strings.TrimSpace(req.AgentName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_name is required"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
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
