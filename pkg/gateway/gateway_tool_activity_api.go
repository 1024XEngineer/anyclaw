package gateway

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleToolActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	writeJSON(w, http.StatusOK, s.store.ListToolActivities(limit, sessionID))
}
