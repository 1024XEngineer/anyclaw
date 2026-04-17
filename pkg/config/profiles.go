package config

import (
	"fmt"
	"os"
	"strings"
)

var mainAgentAliasReplacer = strings.NewReplacer("-", "", "_", "", " ", "")

func (p ProviderProfile) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

func (p AgentProfile) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

func BoolPtr(value bool) *bool {
	return &value
}

func IntPtr(value int) *int {
	return &value
}

func Float64Ptr(value float64) *float64 {
	return &value
}

func normalizeAgentAlias(name string) string {
	return mainAgentAliasReplacer.Replace(strings.ToLower(strings.TrimSpace(name)))
}

func IsMainAgentAlias(name string) bool {
	switch normalizeAgentAlias(name) {
	case "main", "default", "mainagent", "defaultagent":
		return true
	default:
		return false
	}
}

func (c *Config) FindAgentProfile(name string) (AgentProfile, bool) {
	needle := strings.TrimSpace(strings.ToLower(name))
	for _, profile := range c.Agent.Profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) == needle {
			return profile, true
		}
	}
	return AgentProfile{}, false
}

func (c *Config) ResolveMainAgentProfile() (AgentProfile, bool) {
	for _, name := range []string{c.Agent.ActiveProfile, c.Agent.Name} {
		profile, ok := c.FindAgentProfile(name)
		if ok && profile.IsEnabled() {
			return profile, true
		}
	}
	return AgentProfile{}, false
}

func (c *Config) ResolveMainAgentName() string {
	if profile, ok := c.ResolveMainAgentProfile(); ok {
		return profile.Name
	}
	return strings.TrimSpace(c.Agent.Name)
}

func (c *Config) ResolveAgentProfile(name string) (AgentProfile, bool) {
	name = strings.TrimSpace(name)
	if name == "" || IsMainAgentAlias(name) {
		return c.ResolveMainAgentProfile()
	}
	profile, ok := c.FindAgentProfile(name)
	if !ok || !profile.IsEnabled() {
		return AgentProfile{}, false
	}
	return profile, true
}

func (c *Config) IsCurrentAgentProfile(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if profile, ok := c.ResolveMainAgentProfile(); ok && strings.EqualFold(strings.TrimSpace(profile.Name), name) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.Agent.Name), name)
}

func (c *Config) ApplyAgentProfile(name string) bool {
	profile, ok := c.ResolveAgentProfile(name)
	if !ok {
		return false
	}
	if profile.Name != "" {
		c.Agent.Name = profile.Name
		c.Agent.ActiveProfile = profile.Name
	}
	if profile.Description != "" {
		c.Agent.Description = profile.Description
	}
	if profile.WorkingDir != "" {
		c.Agent.WorkingDir = profile.WorkingDir
	}
	if profile.PermissionLevel != "" {
		c.Agent.PermissionLevel = profile.PermissionLevel
	}
	if profile.ProviderRef != "" {
		_ = c.ApplyProviderProfile(profile.ProviderRef)
	}
	if profile.DefaultModel != "" {
		c.LLM.Model = profile.DefaultModel
	}
	if strings.TrimSpace(profile.Persona) != "" {
		c.Agent.Description = strings.TrimSpace(strings.Join([]string{c.Agent.Description, "Persona: " + profile.Persona}, "\n"))
	}
	return true
}

func (c *Config) ApplyAgentRuntimeProfile(name string) bool {
	profile, ok := c.ResolveAgentProfile(name)
	if !ok {
		return false
	}
	if profile.Name != "" {
		c.Agent.Name = profile.Name
		c.Agent.ActiveProfile = profile.Name
	}
	if profile.Description != "" {
		c.Agent.Description = profile.Description
	}
	if profile.WorkingDir != "" {
		c.Agent.WorkingDir = profile.WorkingDir
	}
	if profile.PermissionLevel != "" {
		c.Agent.PermissionLevel = profile.PermissionLevel
	}
	if profile.ProviderRef != "" {
		_ = c.ApplyProviderProfile(profile.ProviderRef)
	}
	if profile.DefaultModel != "" {
		c.LLM.Model = profile.DefaultModel
	}
	return true
}

func (c *Config) UpsertAgentProfile(profile AgentProfile) error {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.Role = strings.TrimSpace(profile.Role)
	profile.Persona = strings.TrimSpace(profile.Persona)
	profile.AvatarPreset = strings.TrimSpace(profile.AvatarPreset)
	profile.AvatarDataURL = strings.TrimSpace(profile.AvatarDataURL)
	profile.Domain = strings.TrimSpace(profile.Domain)
	profile.SystemPrompt = strings.TrimSpace(profile.SystemPrompt)
	profile.WorkingDir = strings.TrimSpace(profile.WorkingDir)
	profile.PermissionLevel = strings.TrimSpace(profile.PermissionLevel)
	profile.ProviderRef = strings.TrimSpace(profile.ProviderRef)
	profile.DefaultModel = strings.TrimSpace(profile.DefaultModel)
	profile.Personality.Template = strings.TrimSpace(profile.Personality.Template)
	profile.Personality.Tone = strings.TrimSpace(profile.Personality.Tone)
	profile.Personality.Style = strings.TrimSpace(profile.Personality.Style)
	profile.Personality.GoalOrientation = strings.TrimSpace(profile.Personality.GoalOrientation)
	profile.Personality.ConstraintMode = strings.TrimSpace(profile.Personality.ConstraintMode)
	profile.Personality.ResponseVerbosity = strings.TrimSpace(profile.Personality.ResponseVerbosity)
	profile.Personality.CustomInstructions = strings.TrimSpace(profile.Personality.CustomInstructions)
	for i, trait := range profile.Personality.Traits {
		profile.Personality.Traits[i] = strings.TrimSpace(trait)
	}
	filteredTraits := make([]string, 0, len(profile.Personality.Traits))
	for _, trait := range profile.Personality.Traits {
		if trait != "" {
			filteredTraits = append(filteredTraits, trait)
		}
	}
	profile.Personality.Traits = filteredTraits
	filteredExpertise := make([]string, 0, len(profile.Expertise))
	for _, e := range profile.Expertise {
		e = strings.TrimSpace(e)
		if e != "" {
			filteredExpertise = append(filteredExpertise, e)
		}
	}
	profile.Expertise = filteredExpertise
	filteredSkills := make([]AgentSkillRef, 0, len(profile.Skills))
	for _, skill := range profile.Skills {
		skill.Name = strings.TrimSpace(skill.Name)
		skill.Version = strings.TrimSpace(skill.Version)
		cleanPerms := make([]string, 0, len(skill.Permissions))
		for _, perm := range skill.Permissions {
			perm = strings.TrimSpace(perm)
			if perm != "" {
				cleanPerms = append(cleanPerms, perm)
			}
		}
		skill.Permissions = cleanPerms
		if skill.Name != "" {
			filteredSkills = append(filteredSkills, skill)
		}
	}
	profile.Skills = filteredSkills
	if profile.Name == "" {
		return os.ErrInvalid
	}
	if profile.AvatarDataURL != "" {
		if !strings.HasPrefix(profile.AvatarDataURL, "data:image/") {
			return fmt.Errorf("avatar_data_url must be a data:image/* URL")
		}
		if len(profile.AvatarDataURL) > 2_000_000 {
			return fmt.Errorf("avatar_data_url is too large")
		}
		profile.AvatarPreset = ""
	}
	for i, existing := range c.Agent.Profiles {
		if strings.EqualFold(strings.TrimSpace(existing.Name), profile.Name) {
			c.Agent.Profiles[i] = profile
			return nil
		}
	}
	c.Agent.Profiles = append(c.Agent.Profiles, profile)
	return nil
}

func (c *Config) DeleteAgentProfile(name string) bool {
	needle := strings.TrimSpace(strings.ToLower(name))
	for i, profile := range c.Agent.Profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) != needle {
			continue
		}
		c.Agent.Profiles = append(c.Agent.Profiles[:i], c.Agent.Profiles[i+1:]...)
		if strings.EqualFold(strings.TrimSpace(c.Agent.ActiveProfile), strings.TrimSpace(profile.Name)) {
			c.Agent.ActiveProfile = ""
		}
		return true
	}
	return false
}

func (c *Config) FindProviderProfile(ref string) (ProviderProfile, bool) {
	needle := strings.TrimSpace(strings.ToLower(ref))
	for _, provider := range c.Providers {
		if strings.ToLower(strings.TrimSpace(provider.ID)) == needle || strings.ToLower(strings.TrimSpace(provider.Name)) == needle {
			return provider, true
		}
	}
	return ProviderProfile{}, false
}

func (c *Config) ApplyProviderProfile(ref string) bool {
	provider, ok := c.FindProviderProfile(ref)
	if !ok || !provider.IsEnabled() {
		return false
	}
	if provider.Provider != "" {
		c.LLM.Provider = provider.Provider
	}
	if provider.BaseURL != "" {
		c.LLM.BaseURL = provider.BaseURL
	}
	if provider.APIKey != "" {
		c.LLM.APIKey = provider.APIKey
	}
	if provider.DefaultModel != "" {
		c.LLM.Model = provider.DefaultModel
	}
	if len(provider.Extra) > 0 {
		if c.LLM.Extra == nil {
			c.LLM.Extra = map[string]string{}
		}
		for k, v := range provider.Extra {
			if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
				c.LLM.Extra[k] = v
			}
		}
	}
	return true
}

func (c *Config) FindDefaultProviderProfile() (ProviderProfile, bool) {
	ref := strings.TrimSpace(c.LLM.DefaultProviderRef)
	if ref == "" {
		return ProviderProfile{}, false
	}
	return c.FindProviderProfile(ref)
}

func (c *Config) ApplyDefaultProviderProfile() bool {
	if ref := strings.TrimSpace(c.LLM.DefaultProviderRef); ref != "" {
		return c.ApplyProviderProfile(ref)
	}
	return false
}

func (c *Config) SetDefaultProviderProfile(ref string) bool {
	provider, ok := c.FindProviderProfile(ref)
	if !ok || !provider.IsEnabled() {
		return false
	}
	c.LLM.DefaultProviderRef = provider.ID
	return c.ApplyProviderProfile(provider.ID)
}

func (c *Config) UpsertProviderProfile(provider ProviderProfile) error {
	provider.ID = normalizeProviderID(provider.ID, provider.Name)
	provider.Name = strings.TrimSpace(provider.Name)
	provider.Type = strings.TrimSpace(provider.Type)
	provider.Provider = strings.TrimSpace(provider.Provider)
	provider.BaseURL = strings.TrimSpace(provider.BaseURL)
	provider.APIKey = strings.TrimSpace(provider.APIKey)
	provider.DefaultModel = strings.TrimSpace(provider.DefaultModel)
	filteredCaps := make([]string, 0, len(provider.Capabilities))
	for _, capability := range provider.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability != "" {
			filteredCaps = append(filteredCaps, capability)
		}
	}
	provider.Capabilities = filteredCaps
	if provider.ID == "" || provider.Name == "" || provider.Provider == "" {
		return os.ErrInvalid
	}
	if provider.Extra != nil {
		clean := make(map[string]string, len(provider.Extra))
		for k, v := range provider.Extra {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if k != "" && v != "" {
				clean[k] = v
			}
		}
		provider.Extra = clean
	}
	for i, existing := range c.Providers {
		if strings.EqualFold(strings.TrimSpace(existing.ID), provider.ID) {
			c.Providers[i] = provider
			return nil
		}
	}
	c.Providers = append(c.Providers, provider)
	return nil
}

func (c *Config) DeleteProviderProfile(ref string) bool {
	needle := strings.TrimSpace(strings.ToLower(ref))
	for i, provider := range c.Providers {
		if strings.ToLower(strings.TrimSpace(provider.ID)) != needle && strings.ToLower(strings.TrimSpace(provider.Name)) != needle {
			continue
		}
		deletedID := provider.ID
		c.Providers = append(c.Providers[:i], c.Providers[i+1:]...)
		for idx := range c.Agent.Profiles {
			if strings.EqualFold(strings.TrimSpace(c.Agent.Profiles[idx].ProviderRef), strings.TrimSpace(deletedID)) {
				c.Agent.Profiles[idx].ProviderRef = ""
			}
		}
		return true
	}
	return false
}

func normalizeProviderID(id string, fallbackName string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		id = strings.TrimSpace(strings.ToLower(fallbackName))
	}
	if id == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	id = replacer.Replace(id)
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}
	return strings.Trim(id, "-")
}
