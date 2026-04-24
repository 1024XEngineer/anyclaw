package gateway

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
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
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return nil
	}
	items := make([]providerView, 0, len(s.mainRuntime.Config.Providers))
	defaultRef := strings.TrimSpace(s.mainRuntime.Config.LLM.DefaultProviderRef)
	for _, provider := range s.mainRuntime.Config.Providers {
		items = append(items, providerToView(provider, s.mainRuntime.Config.Agent.Profiles, defaultRef))
	}
	return items
}
