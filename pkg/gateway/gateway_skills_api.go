package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	skillscatalog "github.com/1024XEngineer/anyclaw/pkg/capability/skills"
)

type skillView = skillscatalog.View

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "skills.read") &&
			!HasPermission(UserFromContext(r.Context()), "config.read") &&
			!HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "skills.read"})
			return
		}
		views, err := s.listSkillViews()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "skills.read", "skills", nil)
		writeJSON(w, http.StatusOK, views)
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		var req struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		view, err := s.setSkillEnabled(req.Name, req.Enabled)
		if err != nil {
			statusCode := http.StatusBadRequest
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				statusCode = http.StatusNotFound
			}
			writeJSON(w, statusCode, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "skills.write", view.Name, map[string]any{"enabled": view.Enabled})
		writeJSON(w, http.StatusOK, view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
