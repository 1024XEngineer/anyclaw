package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (s *Server) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !HasPermission(UserFromContext(r.Context()), "config.write") && !HasPermission(UserFromContext(r.Context()), "config.read") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
		return
	}
	var provider config.ProviderProfile
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	enrichProviderForHealthTest(&provider, s.mainRuntime.Config)
	writeJSON(w, http.StatusOK, activeProviderTest(r.Context(), provider))
}

func enrichProviderForHealthTest(provider *config.ProviderProfile, cfg *config.Config) {
	if provider == nil || cfg == nil {
		return
	}
	if strings.TrimSpace(provider.ID) == "" && strings.TrimSpace(provider.Name) != "" {
		provider.ID = provider.Name
	}
	existing, ok := cfg.FindProviderProfile(firstNonEmpty(provider.ID, provider.Name))
	if !ok {
		return
	}
	if strings.TrimSpace(provider.Name) == "" {
		provider.Name = existing.Name
	}
	if strings.TrimSpace(provider.Type) == "" {
		provider.Type = existing.Type
	}
	if strings.TrimSpace(provider.Provider) == "" {
		provider.Provider = existing.Provider
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		provider.BaseURL = existing.BaseURL
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		provider.APIKey = existing.APIKey
	}
	if strings.TrimSpace(provider.DefaultModel) == "" {
		provider.DefaultModel = existing.DefaultModel
	}
	if len(provider.Capabilities) == 0 {
		provider.Capabilities = append([]string{}, existing.Capabilities...)
	}
	if provider.Enabled == nil {
		provider.Enabled = existing.Enabled
	}
	if len(provider.Extra) == 0 && len(existing.Extra) > 0 {
		provider.Extra = map[string]string{}
		for k, v := range existing.Extra {
			provider.Extra[k] = v
		}
	}
}
