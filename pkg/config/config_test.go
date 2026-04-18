package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidateMissingProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Provider = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if !strings.Contains(err.Error(), "llm.provider") {
		t.Fatalf("error should mention llm.provider: %v", err)
	}
}

func TestValidateMissingModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Model = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "llm.model") {
		t.Fatalf("error should mention llm.model: %v", err)
	}
}

func TestValidateTemperature(t *testing.T) {
	tests := []struct {
		temp  float64
		valid bool
	}{
		{0.0, true},
		{0.7, true},
		{2.0, true},
		{-0.1, false},
		{2.1, false},
	}
	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.LLM.Temperature = tt.temp
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("temperature %f should be valid, got error: %v", tt.temp, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("temperature %f should be invalid", tt.temp)
		}
	}
}

func TestValidatePermissionLevel(t *testing.T) {
	validLevels := []string{"full", "limited", "read-only"}
	for _, level := range validLevels {
		cfg := DefaultConfig()
		cfg.Agent.PermissionLevel = level
		if err := cfg.Validate(); err != nil {
			t.Errorf("permission level %q should be valid: %v", level, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Agent.PermissionLevel = "invalid"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid permission level")
	}
	if !strings.Contains(err.Error(), "agent.permission_level") {
		t.Fatalf("error should mention agent.permission_level: %v", err)
	}
}

func TestValidateGatewayPort(t *testing.T) {
	tests := []struct {
		port  int
		valid bool
	}{
		{0, true},
		{8080, true},
		{65535, true},
		{-1, false},
		{65536, false},
	}
	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.Gateway.Port = tt.port
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("port %d should be valid, got error: %v", tt.port, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("port %d should be invalid", tt.port)
		}
	}
}

func TestValidateMemoryFormat(t *testing.T) {
	validFormats := []string{"markdown", "json", "txt"}
	for _, format := range validFormats {
		cfg := DefaultConfig()
		cfg.Memory.Format = format
		if err := cfg.Validate(); err != nil {
			t.Errorf("memory format %q should be valid: %v", format, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Memory.Format = "xml"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid memory format")
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Provider = ""
	cfg.LLM.Model = ""
	cfg.Gateway.Port = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for multiple invalid fields")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "llm.provider") {
		t.Error("error should mention llm.provider")
	}
	if !strings.Contains(errStr, "llm.model") {
		t.Error("error should mention llm.model")
	}
	if !strings.Contains(errStr, "gateway.port") {
		t.Error("error should mention gateway.port")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("loading non-existent file should use defaults: %v", err)
	}
	if cfg.LLM.Provider != DefaultConfig().LLM.Provider {
		t.Error("should use default provider")
	}
}

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("loading valid config should succeed: %v", err)
	}
	if loaded.LLM.Provider != "qwen" {
		t.Errorf("expected provider qwen, got %s", loaded.LLM.Provider)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("{invalid json}"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("error should mention parse failure: %v", err)
	}
}

func TestLoadUTF8BOMConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bom.json")

	cfg := DefaultConfig()
	cfg.Agent.Name = "个人助手"
	data, _ := json.MarshalIndent(cfg, "", "  ")
	data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("expected BOM config to load: %v", err)
	}
	if loaded.Agent.Name != "个人助手" {
		t.Fatalf("expected agent name to survive BOM load, got %q", loaded.Agent.Name)
	}
}

func TestLoadLegacyAliasConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	data := []byte(`{
  "provider": "compatible",
  "model": "demo-model",
  "apiKey": "legacy-key",
  "baseURL": "https://example.invalid/v1",
  "workingDir": "workspace/demo",
  "skillsDir": "skills/custom",
  "pluginsDir": "plugins/custom"
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("expected legacy config to load: %v", err)
	}
	if loaded.LLM.Provider != "compatible" {
		t.Fatalf("expected provider compatible, got %q", loaded.LLM.Provider)
	}
	if loaded.LLM.APIKey != "legacy-key" {
		t.Fatalf("expected API key to migrate, got %q", loaded.LLM.APIKey)
	}
	if loaded.LLM.BaseURL != "https://example.invalid/v1" {
		t.Fatalf("expected base URL to migrate, got %q", loaded.LLM.BaseURL)
	}
	if loaded.Agent.WorkingDir != "workspace/demo" {
		t.Fatalf("expected working dir to migrate, got %q", loaded.Agent.WorkingDir)
	}
	if loaded.Skills.Dir != "skills/custom" {
		t.Fatalf("expected skills dir to migrate, got %q", loaded.Skills.Dir)
	}
	if loaded.Plugins.Dir != "plugins/custom" {
		t.Fatalf("expected plugins dir to migrate, got %q", loaded.Plugins.Dir)
	}
}

func TestResolvePathUsesConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "configs", "anyclaw.json")
	resolved := ResolvePath(configPath, "workflows/demo")
	expected := filepath.Join(dir, "configs", "workflows", "demo")
	if resolved != expected {
		t.Fatalf("expected %q, got %q", expected, resolved)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")

	cfg := DefaultConfig()
	cfg.LLM.Provider = ""
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid config values")
	}
	if !strings.Contains(err.Error(), "llm.provider") {
		t.Fatalf("error should mention validation issue: %v", err)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key-123")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("loading config with env override should succeed: %v", err)
	}
	if loaded.LLM.APIKey != "test-key-123" {
		t.Errorf("expected API key from env, got %s", loaded.LLM.APIKey)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.LLM.Provider = "anthropic"
	cfg.Gateway.Port = 9999
	cfg.Agent.Profiles = []AgentProfile{
		{
			Name:          "hana",
			Description:   "UI copilot",
			AvatarPreset:  "hana",
			AvatarDataURL: "data:image/webp;base64,ZmFrZS1hdmF0YXI=",
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save should succeed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload should succeed: %v", err)
	}
	if loaded.LLM.Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %s", loaded.LLM.Provider)
	}
	if loaded.Gateway.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loaded.Gateway.Port)
	}
	if len(loaded.Agent.Profiles) != 1 {
		t.Fatalf("expected 1 agent profile, got %d", len(loaded.Agent.Profiles))
	}
	if loaded.Agent.Profiles[0].AvatarPreset != "hana" {
		t.Errorf("expected avatar preset hana, got %q", loaded.Agent.Profiles[0].AvatarPreset)
	}
	if loaded.Agent.Profiles[0].AvatarDataURL != "data:image/webp;base64,ZmFrZS1hdmF0YXI=" {
		t.Errorf("expected avatar data url to round-trip, got %q", loaded.Agent.Profiles[0].AvatarDataURL)
	}
}

func TestSaveRelativizesPathsInsideConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	path := filepath.Join(configDir, "anyclaw.json")

	cfg := DefaultConfig()
	cfg.Agent.WorkDir = filepath.Join(configDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(configDir, "workflows", "default")
	cfg.Agent.Profiles = []AgentProfile{
		{
			Name:            "Go Expert",
			Description:     "Go specialist",
			WorkingDir:      filepath.Join(configDir, "workflows", "go"),
			PermissionLevel: "limited",
		},
	}
	cfg.Skills.Dir = filepath.Join(configDir, "skills")
	cfg.Memory.Dir = filepath.Join(configDir, "memory")
	cfg.Plugins.Dir = filepath.Join(configDir, "plugins")
	cfg.Sandbox.BaseDir = filepath.Join(configDir, ".anyclaw", "sandboxes")
	cfg.Security.AuditLog = filepath.Join(configDir, ".anyclaw", "audit", "audit.jsonl")
	cfg.Daemon.PIDFile = filepath.Join(configDir, ".anyclaw", "gateway.pid")
	cfg.Daemon.LogFile = filepath.Join(configDir, ".anyclaw", "gateway.log")
	cfg.Gateway.ControlUI.Root = filepath.Join(configDir, "ui", "dist")
	cfg.Orchestrator.SubAgents = []SubAgentConfig{
		{
			Name:            "worker",
			Description:     "background worker",
			PermissionLevel: "limited",
			WorkingDir:      filepath.Join(configDir, "workflows", "worker"),
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save should succeed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload should succeed: %v", err)
	}

	if loaded.Agent.WorkDir != ".anyclaw" {
		t.Fatalf("expected relative work dir, got %q", loaded.Agent.WorkDir)
	}
	if loaded.Agent.WorkingDir != "workflows/default" {
		t.Fatalf("expected relative working dir, got %q", loaded.Agent.WorkingDir)
	}
	if loaded.Agent.Profiles[0].WorkingDir != "workflows/go" {
		t.Fatalf("expected relative profile working dir, got %q", loaded.Agent.Profiles[0].WorkingDir)
	}
	if loaded.Skills.Dir != "skills" {
		t.Fatalf("expected relative skills dir, got %q", loaded.Skills.Dir)
	}
	if loaded.Memory.Dir != "memory" {
		t.Fatalf("expected relative memory dir, got %q", loaded.Memory.Dir)
	}
	if loaded.Plugins.Dir != "plugins" {
		t.Fatalf("expected relative plugins dir, got %q", loaded.Plugins.Dir)
	}
	if loaded.Sandbox.BaseDir != ".anyclaw/sandboxes" {
		t.Fatalf("expected relative sandbox dir, got %q", loaded.Sandbox.BaseDir)
	}
	if loaded.Security.AuditLog != ".anyclaw/audit/audit.jsonl" {
		t.Fatalf("expected relative audit log, got %q", loaded.Security.AuditLog)
	}
	if loaded.Daemon.PIDFile != ".anyclaw/gateway.pid" {
		t.Fatalf("expected relative pid file, got %q", loaded.Daemon.PIDFile)
	}
	if loaded.Daemon.LogFile != ".anyclaw/gateway.log" {
		t.Fatalf("expected relative log file, got %q", loaded.Daemon.LogFile)
	}
	if loaded.Gateway.ControlUI.Root != "ui/dist" {
		t.Fatalf("expected relative control UI root, got %q", loaded.Gateway.ControlUI.Root)
	}
	if loaded.Orchestrator.SubAgents[0].WorkingDir != "workflows/worker" {
		t.Fatalf("expected relative sub-agent working dir, got %q", loaded.Orchestrator.SubAgents[0].WorkingDir)
	}
}

func TestValidateDefaultProviderRef(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers = []ProviderProfile{
		{
			ID:       "qwen",
			Name:     "Qwen",
			Provider: "qwen",
			Enabled:  BoolPtr(true),
		},
	}
	cfg.LLM.DefaultProviderRef = "qwen"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default provider ref to validate, got %v", err)
	}

	cfg.LLM.DefaultProviderRef = "missing"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing default provider ref")
	}
	if !strings.Contains(err.Error(), "llm.default_provider_ref") {
		t.Fatalf("error should mention llm.default_provider_ref: %v", err)
	}
}

func TestSandboxBackendValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sandbox.Backend = "kubernetes"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid sandbox backend")
	}
	if !strings.Contains(err.Error(), "sandbox.backend") {
		t.Fatalf("error should mention sandbox.backend: %v", err)
	}
}

func TestSandboxExecutionModeValidation(t *testing.T) {
	validModes := []string{"sandbox", "host-reviewed"}
	for _, mode := range validModes {
		cfg := DefaultConfig()
		cfg.Sandbox.ExecutionMode = mode
		if err := cfg.Validate(); err != nil {
			t.Errorf("sandbox.execution_mode %q should be valid: %v", mode, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Sandbox.ExecutionMode = "host"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid sandbox.execution_mode")
	}
	if !strings.Contains(err.Error(), "sandbox.execution_mode") {
		t.Fatalf("error should mention sandbox.execution_mode: %v", err)
	}
}

func TestSubAgentLLMValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Orchestrator.SubAgents = []SubAgentConfig{
		{
			Name:            "worker",
			PermissionLevel: "full",
			LLMMaxTokens:    IntPtr(-1),
			LLMTemperature:  Float64Ptr(3),
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid sub-agent llm overrides")
	}
	if !strings.Contains(err.Error(), "orchestrator.sub_agents[0].llm_max_tokens") {
		t.Fatalf("error should mention llm_max_tokens: %v", err)
	}
	if !strings.Contains(err.Error(), "orchestrator.sub_agents[0].llm_temperature") {
		t.Fatalf("error should mention llm_temperature: %v", err)
	}
}

func TestLoadSubAgentExplicitZeroOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
  "llm": {"provider":"openai","model":"gpt-4o-mini"},
  "agent": {"name":"AnyClaw","description":"test","work_dir":".anyclaw","working_dir":"workflows","permission_level":"limited"},
  "skills": {"dir":"skills","auto_load":true},
  "memory": {"dir":"memory","max_history":100,"format":"markdown","auto_save":true},
  "gateway": {"host":"127.0.0.1","port":18789,"bind":"loopback"},
  "daemon": {"pid_file":".anyclaw/gateway.pid","log_file":".anyclaw/gateway.log"},
  "channels": {},
  "plugins": {"dir":"plugins"},
  "sandbox": {"execution_mode":"sandbox","backend":"local"},
  "security": {},
  "orchestrator": {
    "enabled": true,
    "sub_agents": [
      {"name":"worker","description":"worker","permission_level":"limited","llm_max_tokens":0,"llm_temperature":0}
    ]
  }
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}
	sub := cfg.Orchestrator.SubAgents[0]
	if sub.LLMMaxTokens == nil || *sub.LLMMaxTokens != 0 {
		t.Fatalf("expected explicit llm_max_tokens=0 to be preserved, got %v", sub.LLMMaxTokens)
	}
	if sub.LLMTemperature == nil || *sub.LLMTemperature != 0 {
		t.Fatalf("expected explicit llm_temperature=0 to be preserved, got %v", sub.LLMTemperature)
	}
}

func TestGatewayBindValidation(t *testing.T) {
	validBinds := []string{"", "loopback", "all", "127.0.0.1", "0.0.0.0", "::1"}
	for _, bind := range validBinds {
		cfg := DefaultConfig()
		cfg.Gateway.Bind = bind
		if err := cfg.Validate(); err != nil {
			t.Errorf("gateway.bind %q should be valid: %v", bind, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Gateway.Bind = "invalid-value"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid gateway.bind")
	}
	if !strings.Contains(err.Error(), "gateway.bind") {
		t.Fatalf("error should mention gateway.bind: %v", err)
	}
}

func TestGatewayControlUIBasePathValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gateway.ControlUI.BasePath = "/console"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected /console base path to be valid: %v", err)
	}

	cfg.Gateway.ControlUI.BasePath = "/"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for gateway.control_ui.base_path=/")
	}
	if !strings.Contains(err.Error(), "gateway.control_ui.base_path") {
		t.Fatalf("error should mention gateway.control_ui.base_path: %v", err)
	}
}

func TestLoadControlUIConfigNormalizesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
  "gateway": {
    "control_ui": {
      "base_path": "console/",
      "root": "./dist/control-ui"
    }
  }
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := cfg.Gateway.ControlUI.BasePath; got != "/console" {
		t.Fatalf("expected normalized base path /console, got %q", got)
	}
	if got := cfg.Gateway.ControlUI.Root; got != "dist/control-ui" {
		t.Fatalf("expected normalized root dist/control-ui, got %q", got)
	}
}

func TestResolveMainAgentProfileFallsBackToAgentName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.Name = "Go Expert"
	cfg.Agent.ActiveProfile = ""
	cfg.Agent.Profiles = []AgentProfile{
		{
			Name:            "Go Expert",
			Description:     "Go specialist",
			PermissionLevel: "limited",
			Enabled:         BoolPtr(true),
		},
	}

	profile, ok := cfg.ResolveMainAgentProfile()
	if !ok {
		t.Fatal("expected main agent profile to resolve from agent.name")
	}
	if profile.Name != "Go Expert" {
		t.Fatalf("expected Go Expert, got %q", profile.Name)
	}
	if got := cfg.ResolveMainAgentName(); got != "Go Expert" {
		t.Fatalf("expected resolved main agent name Go Expert, got %q", got)
	}
	if !cfg.IsCurrentAgentProfile("Go Expert") {
		t.Fatal("expected Go Expert to be marked as the current agent profile")
	}
}

func TestResolveAgentProfileSupportsMainAgentAlias(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.Name = "Python Expert"
	cfg.Agent.Profiles = []AgentProfile{
		{
			Name:            "Python Expert",
			Description:     "Python specialist",
			PermissionLevel: "limited",
			Enabled:         BoolPtr(true),
		},
	}

	profile, ok := cfg.ResolveAgentProfile("mainagent")
	if !ok {
		t.Fatal("expected mainagent alias to resolve")
	}
	if profile.Name != "Python Expert" {
		t.Fatalf("expected Python Expert, got %q", profile.Name)
	}
	if !cfg.ApplyAgentProfile("main-agent") {
		t.Fatal("expected ApplyAgentProfile to accept main-agent alias")
	}
	if cfg.Agent.ActiveProfile != "Python Expert" {
		t.Fatalf("expected active profile Python Expert, got %q", cfg.Agent.ActiveProfile)
	}
}
