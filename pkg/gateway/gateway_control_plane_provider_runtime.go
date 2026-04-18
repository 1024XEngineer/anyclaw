package gateway

import (
	"fmt"
	"strings"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (s *Server) currentDefaultProvider() (config.ProviderProfile, bool) {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return config.ProviderProfile{}, false
	}
	return s.mainRuntime.Config.FindDefaultProviderProfile()
}

func (s *Server) applyDefaultProvider(ref string) (config.ProviderProfile, error) {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return config.ProviderProfile{}, fmt.Errorf("server is not initialized")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return config.ProviderProfile{}, fmt.Errorf("provider_ref is required")
	}
	provider, ok := s.mainRuntime.Config.FindProviderProfile(ref)
	if !ok {
		return config.ProviderProfile{}, fmt.Errorf("provider not found")
	}
	if !provider.IsEnabled() {
		return config.ProviderProfile{}, fmt.Errorf("provider is disabled")
	}
	if !s.mainRuntime.Config.SetDefaultProviderProfile(provider.ID) {
		return config.ProviderProfile{}, fmt.Errorf("unable to apply provider")
	}
	client, err := llm.NewClientWrapper(llm.Config{
		Provider:    s.mainRuntime.Config.LLM.Provider,
		Model:       s.mainRuntime.Config.LLM.Model,
		APIKey:      s.mainRuntime.Config.LLM.APIKey,
		BaseURL:     s.mainRuntime.Config.LLM.BaseURL,
		Proxy:       s.mainRuntime.Config.LLM.Proxy,
		MaxTokens:   s.mainRuntime.Config.LLM.MaxTokens,
		Temperature: s.mainRuntime.Config.LLM.Temperature,
	})
	if err != nil {
		return config.ProviderProfile{}, err
	}
	s.mainRuntime.SetLLMClient(client)
	if s.tasks != nil {
		s.tasks.SetPlanner(client)
	}
	if s.runtimePool != nil {
		s.runtimePool.InvalidateAll()
	}
	updated, _ := s.mainRuntime.Config.FindProviderProfile(provider.ID)
	return updated, nil
}

func (s *Server) invalidateProviderConsumers(providerID string) {
	for _, profile := range s.mainRuntime.Config.Agent.Profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ProviderRef), strings.TrimSpace(providerID)) {
			s.runtimePool.InvalidateByAgent(profile.Name)
		}
	}
}
