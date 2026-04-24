package gateway

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type agentProfileView struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Role            string                 `json:"role,omitempty"`
	Persona         string                 `json:"persona,omitempty"`
	AvatarPreset    string                 `json:"avatar_preset,omitempty"`
	AvatarDataURL   string                 `json:"avatar_data_url,omitempty"`
	WorkingDir      string                 `json:"working_dir,omitempty"`
	PermissionLevel string                 `json:"permission_level,omitempty"`
	ProviderRef     string                 `json:"provider_ref,omitempty"`
	ProviderName    string                 `json:"provider_name,omitempty"`
	ProviderType    string                 `json:"provider_type,omitempty"`
	Provider        string                 `json:"provider,omitempty"`
	DefaultModel    string                 `json:"default_model,omitempty"`
	Enabled         bool                   `json:"enabled"`
	Active          bool                   `json:"active"`
	Personality     config.PersonalitySpec `json:"personality,omitempty"`
	Skills          []config.AgentSkillRef `json:"skills,omitempty"`
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

func (s *Server) listAgentBindingViews() []agentBindingView {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return nil
	}
	items := make([]agentBindingView, 0, len(s.mainRuntime.Config.Agent.Profiles))
	for _, profile := range s.mainRuntime.Config.Agent.Profiles {
		items = append(items, s.buildAgentBindingView(profile))
	}
	return items
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
		Model:           firstNonEmpty(profile.DefaultModel, s.mainRuntime.Config.LLM.Model),
		Skills:          append([]config.AgentSkillRef{}, profile.Skills...),
		Active:          s.mainRuntime.Config.IsCurrentAgentProfile(profile.Name),
		RoutingMode:     "override",
	}
	if provider, ok := s.mainRuntime.Config.FindProviderProfile(profile.ProviderRef); ok {
		view.ResolvedProviderRef = provider.ID
		view.ProviderName = provider.Name
		view.ProviderType = provider.Type
		view.Provider = provider.Provider
		if strings.TrimSpace(profile.DefaultModel) == "" {
			view.Model = firstNonEmpty(provider.DefaultModel, s.mainRuntime.Config.LLM.Model)
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
			view.Model = firstNonEmpty(provider.DefaultModel, s.mainRuntime.Config.LLM.Model)
		}
		view.Health = quickProviderHealth(provider)
	} else {
		view.ProviderName = "Legacy Global"
		view.Provider = s.mainRuntime.Config.LLM.Provider
		view.ProviderType = "global"
		view.InheritsDefault = true
		view.RoutingMode = "legacy"
		view.Health = providerHealth{OK: true, Status: "global_default", Message: "Using legacy global runtime provider."}
	}
	return view
}

func (s *Server) buildAgentProfileView(profile config.AgentProfile) agentProfileView {
	personality := profile.Personality
	if strings.TrimSpace(personality.Template) == "" &&
		len(personality.Traits) == 0 &&
		strings.TrimSpace(personality.Tone) == "" &&
		strings.TrimSpace(personality.Style) == "" {
		personality = config.DefaultPersonalitySpec()
	}
	providerName := ""
	providerType := ""
	providerRuntime := ""
	if provider, ok := s.mainRuntime.Config.FindProviderProfile(profile.ProviderRef); ok {
		providerName = provider.Name
		providerType = provider.Type
		providerRuntime = provider.Provider
	}
	return agentProfileView{
		Name:            profile.Name,
		Description:     profile.Description,
		Role:            profile.Role,
		Persona:         profile.Persona,
		AvatarPreset:    profile.AvatarPreset,
		AvatarDataURL:   profile.AvatarDataURL,
		WorkingDir:      profile.WorkingDir,
		PermissionLevel: profile.PermissionLevel,
		ProviderRef:     profile.ProviderRef,
		ProviderName:    providerName,
		ProviderType:    providerType,
		Provider:        firstNonEmpty(providerRuntime, s.mainRuntime.Config.LLM.Provider),
		DefaultModel:    profile.DefaultModel,
		Enabled:         profile.IsEnabled(),
		Active:          s.mainRuntime.Config.IsCurrentAgentProfile(profile.Name),
		Personality:     personality,
		Skills:          append([]config.AgentSkillRef{}, profile.Skills...),
	}
}

func (s *Server) listAgentViews() []agentProfileView {
	items := make([]agentProfileView, 0, len(s.mainRuntime.Config.Agent.Profiles))
	for _, profile := range s.mainRuntime.Config.Agent.Profiles {
		items = append(items, s.buildAgentProfileView(profile))
	}
	return items
}

func (s *Server) getAgentView(name string) (agentProfileView, bool) {
	for _, profile := range s.mainRuntime.Config.Agent.Profiles {
		if profile.Name == name {
			return s.buildAgentProfileView(profile), true
		}
	}
	return agentProfileView{}, false
}
