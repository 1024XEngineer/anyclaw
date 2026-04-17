package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
		}
	} else {
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err == nil {
			applyLegacyAliases(cfg, raw)
		}
	}

	applyEnvOverrides(cfg)
	normalizeLoadedConfig(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c.persistableCopy(path), "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func (c *Config) persistableCopy(path string) *Config {
	if c == nil {
		return nil
	}

	snapshot := *c
	snapshot.Agent.WorkDir = persistConfigPath(path, snapshot.Agent.WorkDir)
	snapshot.Agent.WorkingDir = persistConfigPath(path, snapshot.Agent.WorkingDir)
	for i := range snapshot.Agent.Profiles {
		snapshot.Agent.Profiles[i].WorkingDir = persistConfigPath(path, snapshot.Agent.Profiles[i].WorkingDir)
	}
	snapshot.Skills.Dir = persistConfigPath(path, snapshot.Skills.Dir)
	snapshot.Memory.Dir = persistConfigPath(path, snapshot.Memory.Dir)
	snapshot.Plugins.Dir = persistConfigPath(path, snapshot.Plugins.Dir)
	snapshot.Sandbox.BaseDir = persistConfigPath(path, snapshot.Sandbox.BaseDir)
	snapshot.Security.AuditLog = persistConfigPath(path, snapshot.Security.AuditLog)
	snapshot.Daemon.PIDFile = persistConfigPath(path, snapshot.Daemon.PIDFile)
	snapshot.Daemon.LogFile = persistConfigPath(path, snapshot.Daemon.LogFile)
	snapshot.Gateway.ControlUI.Root = persistConfigPath(path, snapshot.Gateway.ControlUI.Root)
	for i := range snapshot.Orchestrator.SubAgents {
		snapshot.Orchestrator.SubAgents[i].WorkingDir = persistConfigPath(path, snapshot.Orchestrator.SubAgents[i].WorkingDir)
	}
	return &snapshot
}

func persistConfigPath(configPath string, value string) string {
	cleaned := cleanConfigPath(value)
	if cleaned == "" {
		return ""
	}

	native := filepath.Clean(filepath.FromSlash(cleaned))
	if !filepath.IsAbs(native) {
		return cleaned
	}

	baseDir := filepath.Dir(ResolveConfigPath(configPath))
	rel, err := filepath.Rel(baseDir, native)
	if err != nil {
		return filepath.ToSlash(native)
	}

	rel = filepath.Clean(rel)
	parentPrefix := ".." + string(filepath.Separator)
	if rel == ".." || strings.HasPrefix(rel, parentPrefix) {
		return filepath.ToSlash(native)
	}

	return filepath.ToSlash(rel)
}
