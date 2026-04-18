package runtime

import (
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	runtimebootstrap "github.com/1024XEngineer/anyclaw/pkg/runtime/bootstrap"
)

func resolveMainAgentPersonality(cfg *config.Config) config.PersonalitySpec {
	return runtimebootstrap.ResolveMainAgentPersonality(cfg)
}

func configuredAgentSkillNames(cfg *config.Config) []string {
	return runtimebootstrap.ConfiguredAgentSkillNames(cfg)
}

func enabledSkillNames(items []config.AgentSkillRef) []string {
	return runtimebootstrap.EnabledSkillNames(items)
}

func filterConfiguredSkills(manager *skills.SkillsManager, configured []string) (*skills.SkillsManager, []string) {
	return runtimebootstrap.FilterConfiguredSkills(manager, configured)
}
