package config

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var windowsEnvPattern = regexp.MustCompile(`%([^%]+)%`)

func applyLegacyAliases(cfg *Config, raw map[string]any) {
	if cfg == nil || raw == nil {
		return
	}

	if llmMap, ok := nestedMap(raw, "llm"); ok {
		if value, ok := readStringAlias(llmMap, "apiKey", "api_key"); ok {
			cfg.LLM.APIKey = value
		}
		if value, ok := readStringAlias(llmMap, "baseURL", "base_url"); ok {
			cfg.LLM.BaseURL = value
		}
		if value, ok := readStringAlias(llmMap, "defaultProviderRef", "default_provider_ref"); ok {
			cfg.LLM.DefaultProviderRef = value
		}
		if value, ok := readIntAlias(llmMap, "maxTokens", "max_tokens"); ok {
			cfg.LLM.MaxTokens = value
		}
		if value, ok := readFloatAlias(llmMap, "temperature"); ok {
			cfg.LLM.Temperature = value
		}
	}

	if agentMap, ok := nestedMap(raw, "agent"); ok {
		if value, ok := readStringAlias(agentMap, "workDir", "work_dir"); ok {
			cfg.Agent.WorkDir = value
		}
		if value, ok := readStringAlias(agentMap, "workingDir", "working_dir", "workspace"); ok {
			cfg.Agent.WorkingDir = value
		}
		if value, ok := readStringAlias(agentMap, "permissionLevel", "permission_level"); ok {
			cfg.Agent.PermissionLevel = value
		}
	}

	if skillsMap, ok := nestedMap(raw, "skills"); ok {
		if value, ok := readStringAlias(skillsMap, "skillsDir", "dir", "path"); ok {
			cfg.Skills.Dir = value
		}
	}

	if pluginsMap, ok := nestedMap(raw, "plugins"); ok {
		if value, ok := readStringAlias(pluginsMap, "pluginsDir", "dir", "path"); ok {
			cfg.Plugins.Dir = value
		}
	}

	if memoryMap, ok := nestedMap(raw, "memory"); ok {
		if value, ok := readStringAlias(memoryMap, "memoryDir", "dir", "path"); ok {
			cfg.Memory.Dir = value
		}
	}

	if gatewayMap, ok := nestedMap(raw, "gateway"); ok {
		if value, ok := readIntAlias(gatewayMap, "gatewayPort", "port"); ok {
			cfg.Gateway.Port = value
		}
		if value, ok := readStringAlias(gatewayMap, "gatewayHost", "host"); ok {
			cfg.Gateway.Host = value
		}
		if value, ok := readStringAlias(gatewayMap, "dashboardPath", "dashboard_path"); ok {
			cfg.Gateway.ControlUI.BasePath = value
		}
		if controlUIMap, ok := nestedMap(gatewayMap, "control_ui"); ok {
			if value, ok := readStringAlias(controlUIMap, "basePath", "base_path"); ok {
				cfg.Gateway.ControlUI.BasePath = value
			}
			if value, ok := readStringAlias(controlUIMap, "root"); ok {
				cfg.Gateway.ControlUI.Root = value
			}
		}
		if controlUIMap, ok := nestedMap(gatewayMap, "controlUi"); ok {
			if value, ok := readStringAlias(controlUIMap, "basePath", "base_path"); ok {
				cfg.Gateway.ControlUI.BasePath = value
			}
			if value, ok := readStringAlias(controlUIMap, "root"); ok {
				cfg.Gateway.ControlUI.Root = value
			}
		}
	}

	if value, ok := readStringAlias(raw, "provider"); ok {
		cfg.LLM.Provider = value
	}
	if value, ok := readStringAlias(raw, "model"); ok {
		cfg.LLM.Model = value
	}
	if value, ok := readStringAlias(raw, "apiKey", "api_key"); ok {
		cfg.LLM.APIKey = value
	}
	if value, ok := readStringAlias(raw, "baseURL", "base_url"); ok {
		cfg.LLM.BaseURL = value
	}
	if value, ok := readStringAlias(raw, "workDir", "work_dir"); ok {
		cfg.Agent.WorkDir = value
	}
	if value, ok := readStringAlias(raw, "workingDir", "working_dir", "workspace"); ok {
		cfg.Agent.WorkingDir = value
	}
	if value, ok := readStringAlias(raw, "skillsDir", "skills_dir"); ok {
		cfg.Skills.Dir = value
	}
	if value, ok := readStringAlias(raw, "pluginsDir", "plugins_dir"); ok {
		cfg.Plugins.Dir = value
	}
	if value, ok := readStringAlias(raw, "memoryDir", "memory_dir"); ok {
		cfg.Memory.Dir = value
	}
}

func normalizeLoadedConfig(cfg *Config) {
	if cfg == nil {
		return
	}

	cfg.LLM.Provider = strings.TrimSpace(cfg.LLM.Provider)
	cfg.LLM.Model = strings.TrimSpace(cfg.LLM.Model)
	cfg.LLM.APIKey = strings.TrimSpace(cfg.LLM.APIKey)
	cfg.LLM.BaseURL = strings.TrimSpace(cfg.LLM.BaseURL)
	cfg.LLM.DefaultProviderRef = strings.TrimSpace(cfg.LLM.DefaultProviderRef)
	cfg.LLM.Proxy = strings.TrimSpace(cfg.LLM.Proxy)
	cfg.Agent.Name = strings.TrimSpace(cfg.Agent.Name)
	cfg.Agent.Description = strings.TrimSpace(cfg.Agent.Description)
	cfg.Agent.PermissionLevel = strings.TrimSpace(cfg.Agent.PermissionLevel)
	cfg.Agent.ActiveProfile = strings.TrimSpace(cfg.Agent.ActiveProfile)

	cfg.Agent.WorkDir = cleanConfigPath(cfg.Agent.WorkDir)
	cfg.Agent.WorkingDir = cleanConfigPath(cfg.Agent.WorkingDir)
	cfg.Skills.Dir = cleanConfigPath(cfg.Skills.Dir)
	cfg.Memory.Dir = cleanConfigPath(cfg.Memory.Dir)
	cfg.Plugins.Dir = cleanConfigPath(cfg.Plugins.Dir)
	cfg.Sandbox.BaseDir = cleanConfigPath(cfg.Sandbox.BaseDir)
	cfg.Daemon.PIDFile = cleanConfigPath(cfg.Daemon.PIDFile)
	cfg.Daemon.LogFile = cleanConfigPath(cfg.Daemon.LogFile)
	cfg.Security.AuditLog = cleanConfigPath(cfg.Security.AuditLog)
	cfg.Gateway.ControlUI.BasePath = normalizeControlUIBasePath(cfg.Gateway.ControlUI.BasePath)
	cfg.Gateway.ControlUI.Root = cleanConfigPath(cfg.Gateway.ControlUI.Root)

	for i := range cfg.Providers {
		cfg.Providers[i].ID = normalizeProviderID(cfg.Providers[i].ID, cfg.Providers[i].Name)
		cfg.Providers[i].Name = strings.TrimSpace(cfg.Providers[i].Name)
		cfg.Providers[i].Provider = strings.TrimSpace(cfg.Providers[i].Provider)
		cfg.Providers[i].Type = strings.TrimSpace(cfg.Providers[i].Type)
		cfg.Providers[i].BaseURL = strings.TrimSpace(cfg.Providers[i].BaseURL)
		cfg.Providers[i].APIKey = strings.TrimSpace(cfg.Providers[i].APIKey)
		cfg.Providers[i].DefaultModel = strings.TrimSpace(cfg.Providers[i].DefaultModel)
	}

	if strings.TrimSpace(cfg.LLM.DefaultProviderRef) == "" && len(cfg.Providers) == 1 {
		cfg.LLM.DefaultProviderRef = cfg.Providers[0].ID
	}
}

func normalizeControlUIBasePath(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "/dashboard"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = path.Clean(base)
	if base == "." || base == "/" {
		return "/dashboard"
	}
	return strings.TrimRight(base, "/")
}

func ResolvePath(configPath string, value string) string {
	cleaned := cleanConfigPath(value)
	if cleaned == "" {
		return ""
	}
	native := filepath.Clean(filepath.FromSlash(cleaned))
	if filepath.IsAbs(native) {
		return native
	}
	baseDir := filepath.Dir(ResolveConfigPath(configPath))
	return filepath.Clean(filepath.Join(baseDir, native))
}

func ResolveConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "anyclaw.json"
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func cleanConfigPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = expandPathEnv(value)
	native := filepath.Clean(filepath.FromSlash(value))
	return filepath.ToSlash(native)
}

func expandPathEnv(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = windowsEnvPattern.ReplaceAllStringFunc(value, func(match string) string {
		name := strings.TrimSpace(match[1 : len(match)-1])
		if name == "" {
			return match
		}
		if resolved := os.Getenv(name); resolved != "" {
			return resolved
		}
		return match
	})
	value = os.ExpandEnv(value)
	if strings.HasPrefix(value, "~") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			switch {
			case value == "~":
				value = home
			case strings.HasPrefix(value, "~/"), strings.HasPrefix(value, `~\`):
				value = filepath.Join(home, value[2:])
			}
		}
	}
	return value
}

func nestedMap(raw map[string]any, key string) (map[string]any, bool) {
	value, ok := raw[key]
	if !ok {
		return nil, false
	}
	out, ok := value.(map[string]any)
	return out, ok
}

func readStringAlias(raw map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed), true
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64), true
		case bool:
			if typed {
				return "true", true
			}
			return "false", true
		}
	}
	return "", false
}

func readIntAlias(raw map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed), true
		case int:
			return typed, true
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func readFloatAlias(raw map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed, true
		case int:
			return float64(typed), true
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}
