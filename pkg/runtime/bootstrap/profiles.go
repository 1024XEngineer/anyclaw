package bootstrap

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func ResolveMainAgentPersonality(cfg *config.Config) config.PersonalitySpec {
	if cfg == nil {
		return config.PersonalitySpec{}
	}
	if profile, ok := cfg.ResolveMainAgentProfile(); ok {
		return profile.Personality
	}
	return config.PersonalitySpec{}
}

func ConfiguredAgentSkillNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	if profile, ok := cfg.ResolveMainAgentProfile(); ok {
		return EnabledSkillNames(profile.Skills)
	}
	return EnabledSkillNames(cfg.Agent.Skills)
}

func EnabledSkillNames(skillsCfg []config.AgentSkillRef) []string {
	if len(skillsCfg) == 0 {
		return nil
	}
	items := make([]string, 0, len(skillsCfg))
	seen := make(map[string]struct{}, len(skillsCfg))
	for _, skill := range skillsCfg {
		if !skill.Enabled {
			continue
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, name)
	}
	return items
}

func FilterConfiguredSkills(manager *skills.SkillsManager, configured []string) (*skills.SkillsManager, []string) {
	if manager == nil || len(configured) == 0 {
		return manager, nil
	}
	filtered := manager.FilterEnabled(configured)
	loaded := make(map[string]struct{}, len(filtered.List()))
	for _, skill := range filtered.List() {
		if skill == nil {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(skill.Name))
		if name != "" {
			loaded[name] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, name := range configured {
		key := strings.TrimSpace(strings.ToLower(name))
		if key == "" {
			continue
		}
		if _, ok := loaded[key]; ok {
			continue
		}
		missing = append(missing, name)
	}
	return filtered, missing
}
