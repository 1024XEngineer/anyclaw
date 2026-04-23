package bootstrap

import (
	"path/filepath"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func ResolveRuntimePaths(cfg *config.Config, configPath string) {
	if cfg == nil {
		return
	}
	if resolved := config.ResolvePath(configPath, cfg.Agent.WorkDir); resolved != "" {
		cfg.Agent.WorkDir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Agent.WorkingDir); resolved != "" {
		cfg.Agent.WorkingDir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Skills.Dir); resolved != "" {
		cfg.Skills.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Plugins.Dir); resolved != "" {
		cfg.Plugins.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Memory.Dir); resolved != "" {
		cfg.Memory.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Security.AuditLog); resolved != "" {
		cfg.Security.AuditLog = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Sandbox.BaseDir); resolved != "" {
		cfg.Sandbox.BaseDir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Daemon.PIDFile); resolved != "" {
		cfg.Daemon.PIDFile = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Daemon.LogFile); resolved != "" {
		cfg.Daemon.LogFile = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Gateway.ControlUI.Root); resolved != "" {
		cfg.Gateway.ControlUI.Root = resolved
	}
	for i := range cfg.Agent.Profiles {
		if resolved := config.ResolvePath(configPath, cfg.Agent.Profiles[i].WorkingDir); resolved != "" {
			cfg.Agent.Profiles[i].WorkingDir = resolved
		}
	}
	for i := range cfg.Orchestrator.SubAgents {
		if resolved := config.ResolvePath(configPath, cfg.Orchestrator.SubAgents[i].WorkingDir); resolved != "" {
			cfg.Orchestrator.SubAgents[i].WorkingDir = resolved
		}
	}
}

func ResolveConfigPath(path string) string {
	if path == "" {
		path = "anyclaw.json"
	}
	if filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
