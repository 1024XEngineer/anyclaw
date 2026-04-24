package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleAgentBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") && !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		writeJSON(w, http.StatusOK, s.listAgentBindingViews())
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		var req struct {
			Agent       string   `json:"agent"`
			Agents      []string `json:"agents"`
			ProviderRef string   `json:"provider_ref"`
			Model       *string  `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if strings.TrimSpace(req.Agent) != "" {
			req.Agents = append(req.Agents, req.Agent)
		}
		if len(req.Agents) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent or agents is required"})
			return
		}
		providerRef := strings.TrimSpace(req.ProviderRef)
		if providerRef != "" {
			if _, ok := s.mainRuntime.Config.FindProviderProfile(providerRef); !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider not found"})
				return
			}
		}
		model := ""
		modelProvided := req.Model != nil
		if modelProvided {
			model = strings.TrimSpace(*req.Model)
		}
		updated := make([]agentBindingView, 0, len(req.Agents))
		for _, name := range req.Agents {
			profile, ok := s.mainRuntime.Config.FindAgentProfile(name)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("agent not found: %s", name)})
				return
			}
			profile.ProviderRef = providerRef
			if modelProvided {
				profile.DefaultModel = model
			}
			if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.runtimePool.InvalidateByAgent(profile.Name)
			updated = append(updated, s.buildAgentBindingView(profile))
		}
		if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agent-bindings.write", strings.Join(req.Agents, ","), map[string]any{"provider_ref": providerRef, "model": model})
		writeJSON(w, http.StatusOK, updated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
