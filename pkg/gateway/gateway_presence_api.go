package gateway

import "net/http"

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	if s.presenceMgr == nil {
		http.Error(w, "presence manager not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		ch := r.URL.Query().Get("channel")
		userID := r.URL.Query().Get("user_id")
		if ch != "" && userID != "" {
			info, ok := s.presenceMgr.GetPresence(ch, userID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, info)
		} else {
			writeJSON(w, http.StatusOK, s.presenceMgr.ListActive())
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
