package gateway

import "net/http"

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mem, err := s.mainRuntime.ShowMemory()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"memory": mem})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
