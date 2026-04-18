package config

import (
	"fmt"
	"net"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	if strings.TrimSpace(c.LLM.Provider) == "" {
		errs = append(errs, "llm.provider is required")
	}
	if strings.TrimSpace(c.LLM.Model) == "" {
		errs = append(errs, "llm.model is required")
	}
	if c.LLM.MaxTokens < 0 {
		errs = append(errs, "llm.max_tokens must be >= 0")
	}
	if c.LLM.Temperature < 0 || c.LLM.Temperature > 2.0 {
		errs = append(errs, "llm.temperature must be between 0.0 and 2.0")
	}

	validPermLevels := map[string]bool{"full": true, "limited": true, "read-only": true}
	if c.Agent.PermissionLevel != "" && !validPermLevels[c.Agent.PermissionLevel] {
		errs = append(errs, fmt.Sprintf("agent.permission_level must be one of: full, limited, read-only (got %q)", c.Agent.PermissionLevel))
	}
	for i, provider := range c.Providers {
		if strings.TrimSpace(provider.Name) == "" {
			errs = append(errs, fmt.Sprintf("providers[%d].name is required", i))
		}
		if strings.TrimSpace(provider.Provider) == "" {
			errs = append(errs, fmt.Sprintf("providers[%d].provider is required", i))
		}
	}
	if ref := strings.TrimSpace(c.LLM.DefaultProviderRef); ref != "" {
		if _, ok := c.FindProviderProfile(ref); !ok {
			errs = append(errs, fmt.Sprintf("llm.default_provider_ref must reference an existing provider (got %q)", ref))
		}
	}

	if c.Gateway.Port < 0 || c.Gateway.Port > 65535 {
		errs = append(errs, fmt.Sprintf("gateway.port must be between 0 and 65535 (got %d)", c.Gateway.Port))
	}
	if c.Gateway.Bind != "" && c.Gateway.Bind != "loopback" && c.Gateway.Bind != "all" && net.ParseIP(c.Gateway.Bind) == nil {
		errs = append(errs, fmt.Sprintf("gateway.bind must be 'loopback', 'all', or a valid IP address (got %q)", c.Gateway.Bind))
	}
	if basePath := strings.TrimSpace(c.Gateway.ControlUI.BasePath); basePath != "" {
		if !strings.HasPrefix(basePath, "/") {
			errs = append(errs, fmt.Sprintf("gateway.control_ui.base_path must start with '/' (got %q)", basePath))
		}
		if basePath == "/" {
			errs = append(errs, "gateway.control_ui.base_path cannot be '/'")
		}
	}
	if c.Gateway.RuntimeMaxInstances < 0 {
		errs = append(errs, fmt.Sprintf("gateway.runtime_max_instances must be >= 0 (got %d)", c.Gateway.RuntimeMaxInstances))
	}
	if c.Gateway.JobWorkerCount < 0 {
		errs = append(errs, fmt.Sprintf("gateway.job_worker_count must be >= 0 (got %d)", c.Gateway.JobWorkerCount))
	}

	if c.Memory.MaxHistory < 0 {
		errs = append(errs, fmt.Sprintf("memory.max_history must be >= 0 (got %d)", c.Memory.MaxHistory))
	}
	validFormats := map[string]bool{"markdown": true, "json": true, "txt": true}
	if c.Memory.Format != "" && !validFormats[c.Memory.Format] {
		errs = append(errs, fmt.Sprintf("memory.format must be one of: markdown, json, txt (got %q)", c.Memory.Format))
	}

	if c.Security.RateLimitRPM < 0 {
		errs = append(errs, fmt.Sprintf("security.rate_limit_rpm must be >= 0 (got %d)", c.Security.RateLimitRPM))
	}
	if c.Security.CommandTimeoutSeconds < 0 {
		errs = append(errs, fmt.Sprintf("security.command_timeout_seconds must be >= 0 (got %d)", c.Security.CommandTimeoutSeconds))
	}

	if c.Plugins.ExecTimeoutSeconds < 0 {
		errs = append(errs, fmt.Sprintf("plugins.exec_timeout_seconds must be >= 0 (got %d)", c.Plugins.ExecTimeoutSeconds))
	}

	validBackends := map[string]bool{"local": true, "docker": true}
	if c.Sandbox.Backend != "" && !validBackends[c.Sandbox.Backend] {
		errs = append(errs, fmt.Sprintf("sandbox.backend must be one of: local, docker (got %q)", c.Sandbox.Backend))
	}
	validExecutionModes := map[string]bool{"sandbox": true, "host-reviewed": true}
	if c.Sandbox.ExecutionMode != "" && !validExecutionModes[c.Sandbox.ExecutionMode] {
		errs = append(errs, fmt.Sprintf("sandbox.execution_mode must be one of: sandbox, host-reviewed (got %q)", c.Sandbox.ExecutionMode))
	}
	for i, sa := range c.Orchestrator.SubAgents {
		if sa.PermissionLevel != "" && !validPermLevels[sa.PermissionLevel] {
			errs = append(errs, fmt.Sprintf("orchestrator.sub_agents[%d].permission_level must be one of: full, limited, read-only (got %q)", i, sa.PermissionLevel))
		}
		if sa.LLMMaxTokens != nil && *sa.LLMMaxTokens < 0 {
			errs = append(errs, fmt.Sprintf("orchestrator.sub_agents[%d].llm_max_tokens must be >= 0", i))
		}
		if sa.LLMTemperature != nil && (*sa.LLMTemperature < 0 || *sa.LLMTemperature > 2.0) {
			errs = append(errs, fmt.Sprintf("orchestrator.sub_agents[%d].llm_temperature must be between 0.0 and 2.0", i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
