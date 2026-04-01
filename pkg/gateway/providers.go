package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
)

type providerHealth struct {
	OK         bool   `json:"ok"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type providerView struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Type            string         `json:"type,omitempty"`
	Provider        string         `json:"provider"`
	IsDefault       bool           `json:"is_default"`
	BaseURL         string         `json:"base_url,omitempty"`
	DefaultModel    string         `json:"default_model,omitempty"`
	Capabilities    []string       `json:"capabilities,omitempty"`
	Enabled         bool           `json:"enabled"`
	HasAPIKey       bool           `json:"has_api_key"`
	APIKeyPreview   string         `json:"api_key_preview,omitempty"`
	BoundAgents     []string       `json:"bound_agents,omitempty"`
	BoundAgentCount int            `json:"bound_agent_count"`
	Health          providerHealth `json:"health"`
}

type agentBindingView struct {
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	Role                string                 `json:"role,omitempty"`
	WorkingDir          string                 `json:"working_dir"`
	PermissionLevel     string                 `json:"permission_level"`
	Enabled             bool                   `json:"enabled"`
	ProviderRef         string                 `json:"provider_ref,omitempty"`
	ResolvedProviderRef string                 `json:"resolved_provider_ref,omitempty"`
	ProviderName        string                 `json:"provider_name"`
	ProviderType        string                 `json:"provider_type,omitempty"`
	Provider            string                 `json:"provider"`
	Model               string                 `json:"model"`
	InheritsDefault     bool                   `json:"inherits_default,omitempty"`
	RoutingMode         string                 `json:"routing_mode,omitempty"`
	Health              providerHealth         `json:"health"`
	Skills              []config.AgentSkillRef `json:"skills,omitempty"`
	Active              bool                   `json:"active"`
}

func providerRequiresAPIKey(provider string) bool {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "ollama":
		return false
	default:
		return true
	}
}

func maskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}

func providerToView(provider config.ProviderProfile, profiles []config.AgentProfile, defaultRef string) providerView {
	boundAgents := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ProviderRef), strings.TrimSpace(provider.ID)) {
			boundAgents = append(boundAgents, profile.Name)
		}
	}
	return providerView{
		ID:              provider.ID,
		Name:            provider.Name,
		Type:            provider.Type,
		Provider:        provider.Provider,
		IsDefault:       strings.EqualFold(strings.TrimSpace(defaultRef), strings.TrimSpace(provider.ID)),
		BaseURL:         provider.BaseURL,
		DefaultModel:    provider.DefaultModel,
		Capabilities:    append([]string{}, provider.Capabilities...),
		Enabled:         provider.IsEnabled(),
		HasAPIKey:       strings.TrimSpace(provider.APIKey) != "",
		APIKeyPreview:   maskSecret(provider.APIKey),
		BoundAgents:     boundAgents,
		BoundAgentCount: len(boundAgents),
		Health:          quickProviderHealth(provider),
	}
}

func (s *Server) listProviderViews() []providerView {
	if s == nil || s.app == nil || s.app.Config == nil {
		return nil
	}
	items := make([]providerView, 0, len(s.app.Config.Providers))
	defaultRef := strings.TrimSpace(s.app.Config.LLM.DefaultProviderRef)
	for _, provider := range s.app.Config.Providers {
		items = append(items, providerToView(provider, s.app.Config.Agent.Profiles, defaultRef))
	}
	return items
}

func (s *Server) listAgentBindingViews() []agentBindingView {
	if s == nil || s.app == nil || s.app.Config == nil {
		return nil
	}
	items := make([]agentBindingView, 0, len(s.app.Config.Agent.Profiles))
	for _, profile := range s.app.Config.Agent.Profiles {
		items = append(items, s.buildAgentBindingView(profile))
	}
	return items
}

func quickProviderHealth(provider config.ProviderProfile) providerHealth {
	if !provider.IsEnabled() {
		return providerHealth{Status: "disabled", Message: "Provider is disabled."}
	}
	if strings.TrimSpace(provider.Provider) == "" {
		return providerHealth{Status: "invalid", Message: "Missing runtime provider type."}
	}
	if providerRequiresAPIKey(provider.Provider) && strings.TrimSpace(provider.APIKey) == "" {
		return providerHealth{Status: "missing_key", Message: "API key required."}
	}
	if base := strings.TrimSpace(provider.BaseURL); base != "" {
		if _, err := url.ParseRequestURI(base); err != nil {
			return providerHealth{Status: "invalid_base_url", Message: "Base URL is not a valid URL."}
		}
	}
	return providerHealth{OK: true, Status: "ready", Message: "Ready to use."}
}

func (s *Server) currentDefaultProvider() (config.ProviderProfile, bool) {
	if s == nil || s.app == nil || s.app.Config == nil {
		return config.ProviderProfile{}, false
	}
	return s.app.Config.FindDefaultProviderProfile()
}

func (s *Server) applyDefaultProvider(ref string) (config.ProviderProfile, error) {
	if s == nil || s.app == nil || s.app.Config == nil {
		return config.ProviderProfile{}, fmt.Errorf("server is not initialized")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return config.ProviderProfile{}, fmt.Errorf("provider_ref is required")
	}
	provider, ok := s.app.Config.FindProviderProfile(ref)
	if !ok {
		return config.ProviderProfile{}, fmt.Errorf("provider not found")
	}
	if !provider.IsEnabled() {
		return config.ProviderProfile{}, fmt.Errorf("provider is disabled")
	}
	if !s.app.Config.SetDefaultProviderProfile(provider.ID) {
		return config.ProviderProfile{}, fmt.Errorf("unable to apply provider")
	}
	client, err := llm.NewClientWrapper(llm.Config{
		Provider:    s.app.Config.LLM.Provider,
		Model:       s.app.Config.LLM.Model,
		APIKey:      s.app.Config.LLM.APIKey,
		BaseURL:     s.app.Config.LLM.BaseURL,
		Proxy:       s.app.Config.LLM.Proxy,
		MaxTokens:   s.app.Config.LLM.MaxTokens,
		Temperature: s.app.Config.LLM.Temperature,
	})
	if err != nil {
		return config.ProviderProfile{}, err
	}
	s.app.LLM = client
	if s.tasks != nil {
		s.tasks.planner = client
	}
	if s.runtimePool != nil {
		s.runtimePool.InvalidateAll()
	}
	updated, _ := s.app.Config.FindProviderProfile(provider.ID)
	return updated, nil
}

func activeProviderTest(ctx context.Context, provider config.ProviderProfile) providerHealth {
	initial := quickProviderHealth(provider)
	if !initial.OK && initial.Status != "ready" {
		return initial
	}
	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		if strings.EqualFold(provider.Provider, "ollama") {
			baseURL = "http://127.0.0.1:11434"
		} else {
			return providerHealth{OK: true, Status: "ready", Message: "Using provider default endpoint."}
		}
	}
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return providerHealth{Status: "invalid_base_url", Message: "Base URL is not a valid URL."}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return providerHealth{Status: "request_error", Message: err.Error()}
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return providerHealth{Status: "unreachable", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return providerHealth{OK: true, Status: "reachable", Message: fmt.Sprintf("Endpoint responded with HTTP %d.", resp.StatusCode), HTTPStatus: resp.StatusCode}
	}
	return providerHealth{Status: "error", Message: fmt.Sprintf("Endpoint responded with HTTP %d.", resp.StatusCode), HTTPStatus: resp.StatusCode}
}

func (s *Server) buildAgentBindingView(profile config.AgentProfile) agentBindingView {
	view := agentBindingView{
		Name:            profile.Name,
		Description:     profile.Description,
		Role:            profile.Role,
		WorkingDir:      profile.WorkingDir,
		PermissionLevel: profile.PermissionLevel,
		Enabled:         profile.IsEnabled(),
		ProviderRef:     profile.ProviderRef,
		Model:           firstNonEmpty(profile.DefaultModel, s.app.Config.LLM.Model),
		Skills:          append([]config.AgentSkillRef{}, profile.Skills...),
		Active:          s.app.Config.IsCurrentAgentProfile(profile.Name),
		RoutingMode:     "override",
	}
	if provider, ok := s.app.Config.FindProviderProfile(profile.ProviderRef); ok {
		view.ResolvedProviderRef = provider.ID
		view.ProviderName = provider.Name
		view.ProviderType = provider.Type
		view.Provider = provider.Provider
		if strings.TrimSpace(profile.DefaultModel) == "" {
			view.Model = firstNonEmpty(provider.DefaultModel, s.app.Config.LLM.Model)
		}
		view.Health = quickProviderHealth(provider)
	} else if provider, ok := s.currentDefaultProvider(); ok {
		view.ResolvedProviderRef = provider.ID
		view.ProviderName = provider.Name
		view.ProviderType = provider.Type
		view.Provider = provider.Provider
		view.InheritsDefault = true
		view.RoutingMode = "inherit"
		if strings.TrimSpace(profile.DefaultModel) == "" {
			view.Model = firstNonEmpty(provider.DefaultModel, s.app.Config.LLM.Model)
		}
		view.Health = quickProviderHealth(provider)
	} else {
		view.ProviderName = "Legacy Global"
		view.Provider = s.app.Config.LLM.Provider
		view.ProviderType = "global"
		view.InheritsDefault = true
		view.RoutingMode = "legacy"
		view.Health = providerHealth{OK: true, Status: "global_default", Message: "Using legacy global runtime provider."}
	}
	return view
}

func (s *Server) invalidateProviderConsumers(providerID string) {
	for _, profile := range s.app.Config.Agent.Profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ProviderRef), strings.TrimSpace(providerID)) {
			s.runtimePool.InvalidateByAgent(profile.Name)
		}
	}
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") && !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		writeJSON(w, http.StatusOK, s.listProviderViews())
	case http.MethodPost:
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
		existing, hadExisting := s.app.Config.FindProviderProfile(firstNonEmpty(provider.ID, provider.Name))
		if hadExisting {
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
		wasDefaultRef := strings.TrimSpace(s.app.Config.LLM.DefaultProviderRef)
		if err := s.app.Config.UpsertProviderProfile(provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		updated, _ := s.app.Config.FindProviderProfile(firstNonEmpty(provider.ID, provider.Name))
		if strings.TrimSpace(s.app.Config.LLM.DefaultProviderRef) == "" && updated.IsEnabled() {
			_ = s.app.Config.SetDefaultProviderProfile(updated.ID)
		} else if strings.EqualFold(wasDefaultRef, firstNonEmpty(existing.ID, updated.ID)) || strings.EqualFold(wasDefaultRef, updated.ID) {
			_ = s.app.Config.SetDefaultProviderProfile(updated.ID)
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if strings.EqualFold(strings.TrimSpace(s.app.Config.LLM.DefaultProviderRef), strings.TrimSpace(updated.ID)) {
			if s.runtimePool != nil {
				s.runtimePool.InvalidateAll()
			}
		} else if hadExisting {
			s.invalidateProviderConsumers(existing.ID)
		}
		s.appendAudit(UserFromContext(r.Context()), "providers.write", updated.ID, nil)
		writeJSON(w, http.StatusOK, providerToView(updated, s.app.Config.Agent.Profiles, s.app.Config.LLM.DefaultProviderRef))
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}
		existing, ok := s.app.Config.FindProviderProfile(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
			return
		}
		if strings.EqualFold(strings.TrimSpace(s.app.Config.LLM.DefaultProviderRef), strings.TrimSpace(existing.ID)) && len(s.app.Config.Providers) > 1 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "switch the default provider before deleting it"})
			return
		}
		if !s.app.Config.DeleteProviderProfile(id) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
			return
		}
		if strings.EqualFold(strings.TrimSpace(s.app.Config.LLM.DefaultProviderRef), strings.TrimSpace(existing.ID)) {
			s.app.Config.LLM.DefaultProviderRef = ""
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.invalidateProviderConsumers(existing.ID)
		s.appendAudit(UserFromContext(r.Context()), "providers.delete", existing.ID, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": existing.ID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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
	if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "providers.default", provider.ID, nil)
	writeJSON(w, http.StatusOK, providerToView(provider, s.app.Config.Agent.Profiles, s.app.Config.LLM.DefaultProviderRef))
}

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
	if strings.TrimSpace(provider.ID) == "" && strings.TrimSpace(provider.Name) != "" {
		provider.ID = provider.Name
	}
	if existing, ok := s.app.Config.FindProviderProfile(firstNonEmpty(provider.ID, provider.Name)); ok {
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
	writeJSON(w, http.StatusOK, activeProviderTest(r.Context(), provider))
}

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
			if _, ok := s.app.Config.FindProviderProfile(providerRef); !ok {
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
			profile, ok := s.app.Config.FindAgentProfile(name)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("agent not found: %s", name)})
				return
			}
			profile.ProviderRef = providerRef
			if modelProvided {
				profile.DefaultModel = model
			}
			if err := s.app.Config.UpsertAgentProfile(profile); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.runtimePool.InvalidateByAgent(profile.Name)
			updated = append(updated, s.buildAgentBindingView(profile))
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agent-bindings.write", strings.Join(req.Agents, ","), map[string]any{"provider_ref": providerRef, "model": model})
		writeJSON(w, http.StatusOK, updated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
