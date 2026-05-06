package bootstrap

import (
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/orchestrator"
)

func BuildOrchestratorConfig(cfg *config.Config, workDir string, workingDir string) orchestrator.OrchestratorConfig {
	orchCfg := cfg.Orchestrator

	timeout := time.Duration(orchCfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	defs := make([]orchestrator.AgentDefinition, 0)
	mainPermission := normalizePermissionLevel(cfg.Agent.PermissionLevel)

	for _, agentName := range orchCfg.AgentNames {
		if strings.TrimSpace(agentName) == "" {
			continue
		}
		profile, ok := cfg.FindAgentProfile(agentName)
		if !ok {
			continue
		}
		def := orchestrator.AgentDefinition{
			Name:            profile.Name,
			Description:     profile.Description,
			Persona:         profile.Persona,
			Domain:          profile.Domain,
			Expertise:       profile.Expertise,
			SystemPrompt:    profile.SystemPrompt,
			PrivateSkills:   make([]string, len(profile.Skills)),
			PermissionLevel: clampPermissionLevel(profile.PermissionLevel, mainPermission),
			WorkingDir:      profile.WorkingDir,
		}
		for i, skill := range profile.Skills {
			def.PrivateSkills[i] = skill.Name
		}
		if profile.ProviderRef != "" {
			if provider, ok := cfg.FindProviderProfile(profile.ProviderRef); ok {
				def.LLMProvider = provider.Provider
				def.LLMModel = provider.DefaultModel
				def.LLMAPIKey = provider.APIKey
				def.LLMBaseURL = provider.BaseURL
			}
		}
		if def.WorkingDir == "" {
			def.WorkingDir = workingDir
		}
		defs = append(defs, def)
	}

	for _, saCfg := range orchCfg.SubAgents {
		if strings.TrimSpace(saCfg.Name) == "" {
			continue
		}
		def := resolveSubAgentDefinition(saCfg, cfg.LLM)
		def.PermissionLevel = clampPermissionLevel(def.PermissionLevel, mainPermission)
		if def.WorkingDir == "" {
			def.WorkingDir = workingDir
		}
		defs = append(defs, def)
	}

	return orchestrator.OrchestratorConfig{
		MaxConcurrentAgents: orchCfg.MaxConcurrentAgents,
		MaxRetries:          orchCfg.MaxRetries,
		Timeout:             timeout,
		AgentDefinitions:    defs,
		EnableDecomposition: orchCfg.EnableDecomposition,
		DefaultWorkingDir:   workingDir,
		SkillExecution: &skills.ExecutionOptions{
			AllowExec:          cfg.Plugins.AllowExec,
			ExecTimeoutSeconds: cfg.Plugins.ExecTimeoutSeconds,
		},
	}
}

func resolveSubAgentDefinition(saCfg config.SubAgentConfig, global config.LLMConfig) orchestrator.AgentDefinition {
	def := orchestrator.AgentDefinition{
		Name:            saCfg.Name,
		Description:     saCfg.Description,
		Role:            saCfg.Role,
		ParentRef:       saCfg.ParentRef,
		Persona:         saCfg.Personality,
		Domain:          saCfg.Domain,
		Expertise:       append([]string(nil), saCfg.Expertise...),
		PrivateSkills:   saCfg.PrivateSkills,
		PermissionLevel: saCfg.PermissionLevel,
		WorkingDir:      saCfg.WorkingDir,
		LLMProvider:     saCfg.LLMProvider,
		LLMModel:        saCfg.LLMModel,
		LLMAPIKey:       saCfg.LLMAPIKey,
		LLMBaseURL:      saCfg.LLMBaseURL,
		LLMMaxTokens:    copyIntPtr(saCfg.LLMMaxTokens),
		LLMTemperature:  copyFloat64Ptr(saCfg.LLMTemperature),
		LLMProxy:        saCfg.LLMProxy,
	}
	if def.LLMProvider == "" {
		def.LLMProvider = global.Provider
	}
	if def.LLMModel == "" {
		def.LLMModel = global.Model
	}
	if def.LLMAPIKey == "" {
		def.LLMAPIKey = global.APIKey
	}
	if def.LLMBaseURL == "" {
		def.LLMBaseURL = global.BaseURL
	}
	if def.LLMProxy == "" {
		def.LLMProxy = global.Proxy
	}
	if def.LLMMaxTokens == nil {
		def.LLMMaxTokens = copyIntPtr(&global.MaxTokens)
	}
	if def.LLMTemperature == nil {
		def.LLMTemperature = copyFloat64Ptr(&global.Temperature)
	}
	return def
}

func normalizePermissionLevel(level string) string {
	switch strings.TrimSpace(level) {
	case "full", "limited", "read-only":
		return strings.TrimSpace(level)
	default:
		return "limited"
	}
}

func permissionRank(level string) int {
	switch normalizePermissionLevel(level) {
	case "read-only":
		return 0
	case "limited":
		return 1
	case "full":
		return 2
	default:
		return 1
	}
}

func clampPermissionLevel(requested string, ceiling string) string {
	requested = normalizePermissionLevel(requested)
	ceiling = normalizePermissionLevel(ceiling)
	if permissionRank(requested) > permissionRank(ceiling) {
		return ceiling
	}
	return requested
}

func copyIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
