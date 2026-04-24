package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") && !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		writeJSON(w, http.StatusOK, s.listProviderViews())
	case http.MethodPost:
		s.handleProviderUpsert(w, r)
	case http.MethodDelete:
		s.handleProviderDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProviderUpsert(w http.ResponseWriter, r *http.Request) {
	if !HasPermission(UserFromContext(r.Context()), "config.write") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
		return
	}
	var provider config.ProviderProfile
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if provider.Enabled == nil {
		provider.Enabled = config.BoolPtr(true)
	}
	existing, hadExisting := s.mainRuntime.Config.FindProviderProfile(firstNonEmpty(provider.ID, provider.Name))
	if hadExisting {
		mergeProviderSecretsAndExtra(&provider, existing)
	}
	wasDefaultRef := strings.TrimSpace(s.mainRuntime.Config.LLM.DefaultProviderRef)
	if err := s.mainRuntime.Config.UpsertProviderProfile(provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, _ := s.mainRuntime.Config.FindProviderProfile(firstNonEmpty(provider.ID, provider.Name))
	if strings.TrimSpace(s.mainRuntime.Config.LLM.DefaultProviderRef) == "" && updated.IsEnabled() {
		_ = s.mainRuntime.Config.SetDefaultProviderProfile(updated.ID)
	} else if strings.EqualFold(wasDefaultRef, firstNonEmpty(existing.ID, updated.ID)) || strings.EqualFold(wasDefaultRef, updated.ID) {
		_ = s.mainRuntime.Config.SetDefaultProviderProfile(updated.ID)
	}
	if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if strings.EqualFold(strings.TrimSpace(s.mainRuntime.Config.LLM.DefaultProviderRef), strings.TrimSpace(updated.ID)) {
		if s.runtimePool != nil {
			s.runtimePool.InvalidateAll()
		}
	} else if hadExisting {
		s.invalidateProviderConsumers(existing.ID)
	}
	s.appendAudit(UserFromContext(r.Context()), "providers.write", updated.ID, nil)
	writeJSON(w, http.StatusOK, providerToView(updated, s.mainRuntime.Config.Agent.Profiles, s.mainRuntime.Config.LLM.DefaultProviderRef))
}

func (s *Server) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	if !HasPermission(UserFromContext(r.Context()), "config.write") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	existing, ok := s.mainRuntime.Config.FindProviderProfile(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(s.mainRuntime.Config.LLM.DefaultProviderRef), strings.TrimSpace(existing.ID)) && len(s.mainRuntime.Config.Providers) > 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "switch the default provider before deleting it"})
		return
	}
	if !s.mainRuntime.Config.DeleteProviderProfile(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(s.mainRuntime.Config.LLM.DefaultProviderRef), strings.TrimSpace(existing.ID)) {
		s.mainRuntime.Config.LLM.DefaultProviderRef = ""
	}
	if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.invalidateProviderConsumers(existing.ID)
	s.appendAudit(UserFromContext(r.Context()), "providers.delete", existing.ID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": existing.ID})
}

func mergeProviderSecretsAndExtra(provider *config.ProviderProfile, existing config.ProviderProfile) {
	if provider == nil {
		return
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		provider.APIKey = existing.APIKey
	}
	if len(provider.Extra) == 0 && len(existing.Extra) > 0 {
		provider.Extra = map[string]string{}
		for k, v := range existing.Extra {
			provider.Extra[k] = v
		}
	}
}
