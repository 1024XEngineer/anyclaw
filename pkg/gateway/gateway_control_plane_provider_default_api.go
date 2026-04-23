package gateway

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleDefaultProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !HasPermission(UserFromContext(r.Context()), "config.write") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
		return
	}
	var req struct {
		ProviderRef string `json:"provider_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	provider, err := s.applyDefaultProvider(req.ProviderRef)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "providers.default", provider.ID, nil)
	writeJSON(w, http.StatusOK, providerToView(provider, s.mainRuntime.Config.Agent.Profiles, s.mainRuntime.Config.LLM.DefaultProviderRef))
}
