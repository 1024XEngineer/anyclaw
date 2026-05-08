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

	validSandboxModes := map[string]bool{"read-only": true, "workspace-write": true, "danger-full-access": true}
	if !validSandboxModes[c.Permissions.SandboxMode] {
		errs = append(errs, fmt.Sprintf("permissions.sandbox_mode must be one of: read-only, workspace-write, danger-full-access (got %q)", c.Permissions.SandboxMode))
	}
	validApprovalPolicies := map[string]bool{"untrusted": true, "on-request": true, "on-failure": true, "never": true}
	if !validApprovalPolicies[c.Permissions.ApprovalPolicy] {
		errs = append(errs, fmt.Sprintf("permissions.approval_policy must be one of: untrusted, on-request, on-failure, never (got %q)", c.Permissions.ApprovalPolicy))
	}
	validNetworkAccess := map[string]bool{"enabled": true, "disabled": true}
	if !validNetworkAccess[c.Permissions.NetworkAccess] {
		errs = append(errs, fmt.Sprintf("permissions.network_access must be one of: enabled, disabled (got %q)", c.Permissions.NetworkAccess))
	}
	validDesktopAccess := map[string]bool{"disabled": true, "ask-once-per-session": true, "always": true}
	if !validDesktopAccess[c.Permissions.DesktopAccess] {
		errs = append(errs, fmt.Sprintf("permissions.desktop_access must be one of: disabled, ask-once-per-session, always (got %q)", c.Permissions.DesktopAccess))
	}
	validPermLevels := map[string]bool{"full": true, "limited": true, "read-only": true}
	if !validPermLevels[c.Agent.PermissionLevel] {
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
	if c.Marketplace.RegistryEndpoint != "" {
		if !strings.HasPrefix(c.Marketplace.RegistryEndpoint, "http://") && !strings.HasPrefix(c.Marketplace.RegistryEndpoint, "https://") {
			errs = append(errs, fmt.Sprintf("marketplace.registry_endpoint must start with http:// or https:// (got %q)", c.Marketplace.RegistryEndpoint))
		}
	}
	if c.Marketplace.CacheTTLSeconds < 0 {
		errs = append(errs, fmt.Sprintf("marketplace.cache_ttl_seconds must be >= 0 (got %d)", c.Marketplace.CacheTTLSeconds))
	}
	if c.Marketplace.RequestTimeoutSeconds < 0 {
		errs = append(errs, fmt.Sprintf("marketplace.request_timeout_seconds must be >= 0 (got %d)", c.Marketplace.RequestTimeoutSeconds))
	}
	if c.Marketplace.DownloadTimeoutSeconds < 0 {
		errs = append(errs, fmt.Sprintf("marketplace.download_timeout_seconds must be >= 0 (got %d)", c.Marketplace.DownloadTimeoutSeconds))
	}
	if c.Marketplace.RetryCount < 0 {
		errs = append(errs, fmt.Sprintf("marketplace.retry_count must be >= 0 (got %d)", c.Marketplace.RetryCount))
	}

	validBackends := map[string]bool{"local": true, "docker": true}
	if c.Sandbox.Backend != "" && !validBackends[c.Sandbox.Backend] {
		errs = append(errs, fmt.Sprintf("sandbox.backend must be one of: local, docker (got %q)", c.Sandbox.Backend))
	}
	validExecutionModes := map[string]bool{"sandbox": true, "host-reviewed": true}
	if c.Sandbox.ExecutionMode != "" && !validExecutionModes[c.Sandbox.ExecutionMode] {
		errs = append(errs, fmt.Sprintf("sandbox.execution_mode must be one of: sandbox, host-reviewed (got %q)", c.Sandbox.ExecutionMode))
	}
	validDesktopApprovalScopes := map[string]bool{"capability": true, "tool_call": true}
	if c.Security.DesktopApprovalScope != "" && !validDesktopApprovalScopes[c.Security.DesktopApprovalScope] {
		errs = append(errs, fmt.Sprintf("security.desktop_approval_scope must be one of: capability, tool_call (got %q)", c.Security.DesktopApprovalScope))
	}
	validComputerBackends := map[string]bool{"codex_local": true}
	if c.Computer.Backend != "" && !validComputerBackends[c.Computer.Backend] {
		errs = append(errs, fmt.Sprintf("computer.backend must be codex_local (got %q)", c.Computer.Backend))
	}
	validComputerCoordinateSpaces := map[string]bool{"normalized_0_1000": true, "normalized": true, "absolute": true, "screen": true, "pixels": true}
	if c.Computer.CoordinateSpace != "" && !validComputerCoordinateSpaces[c.Computer.CoordinateSpace] {
		errs = append(errs, fmt.Sprintf("computer.coordinate_space must be one of: normalized_0_1000, normalized, absolute, screen, pixels (got %q)", c.Computer.CoordinateSpace))
	}
	if c.Computer.MaxActionsPerTurn < 0 {
		errs = append(errs, fmt.Sprintf("computer.max_actions_per_turn must be >= 0 (got %d)", c.Computer.MaxActionsPerTurn))
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
