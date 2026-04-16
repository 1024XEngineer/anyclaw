package runtime

import (
	"github.com/anyclaw/anyclaw/pkg/config"
	runtimebootstrap "github.com/anyclaw/anyclaw/pkg/runtime/bootstrap"
	"github.com/anyclaw/anyclaw/pkg/skills"
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
