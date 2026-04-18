package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

func TestNewTargetAppAppliesAgentProfileProviderAndPreservesWorkspaceOverride(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"
	cfg.LLM.APIKey = "global-key"
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workflows", "personal")
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")

	enabled := config.BoolPtr(true)
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "qwen",
			Name:         "Qwen",
			Provider:     "qwen",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:       "provider-key",
			DefaultModel: "qwen-max",
			Enabled:      enabled,
		},
	}
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:            "Go Expert",
			Description:     "Go specialist",
			WorkingDir:      "workflows/go-expert",
			PermissionLevel: "limited",
			ProviderRef:     "qwen",
			DefaultModel:    "qwen-max",
			Enabled:         enabled,
		},
	}

	configPath := filepath.Join(tempDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	workspacePath := filepath.Join(tempDir, "workspace")
	for _, dir := range []string{cfg.Skills.Dir, cfg.Plugins.Dir, workspacePath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	app, err := NewTargetApp(configPath, "Go Expert", workspacePath)
	if err != nil {
		t.Fatalf("NewTargetApp: %v", err)
	}
	t.Cleanup(func() { app.Memory.Close() })

	absWorkspacePath, err := filepath.Abs(workspacePath)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	if app.Config.Agent.ActiveProfile != "Go Expert" {
		t.Fatalf("expected active profile Go Expert, got %q", app.Config.Agent.ActiveProfile)
	}
	if app.Config.Agent.Name != "Go Expert" {
		t.Fatalf("expected agent name Go Expert, got %q", app.Config.Agent.Name)
	}
	if app.Config.LLM.Provider != "qwen" {
		t.Fatalf("expected provider qwen, got %q", app.Config.LLM.Provider)
	}
	if app.Config.LLM.Model != "qwen-max" {
		t.Fatalf("expected model qwen-max, got %q", app.Config.LLM.Model)
	}
	if app.Config.LLM.APIKey != "provider-key" {
		t.Fatalf("expected provider API key to override global LLM key")
	}
	if app.WorkingDir != absWorkspacePath {
		t.Fatalf("expected working dir %q, got %q", absWorkspacePath, app.WorkingDir)
	}
	for _, name := range []string{"AGENTS.md", "SOUL.md", "TOOLS.md", "IDENTITY.md", "USER.md", "HEARTBEAT.md", "MEMORY.md"} {
		if _, err := os.Stat(filepath.Join(absWorkspacePath, name)); err != nil {
			t.Fatalf("expected bootstrap file %s to exist: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(absWorkspacePath, "BOOTSTRAP.md")); err != nil {
		t.Fatalf("expected BOOTSTRAP.md to exist for a new workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(absWorkspacePath, "memory")); err != nil {
		t.Fatalf("expected memory directory to exist: %v", err)
	}
	if err := app.Memory.Add(memory.MemoryEntry{
		ID:        "fact-1",
		Timestamp: filepathModTimeFixture(),
		Type:      memory.TypeFact,
		Content:   "Workspace memory sync is enabled.",
	}); err != nil {
		t.Fatalf("app.Memory.Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(absWorkspacePath, "memory", "2026-03-29.md")); err != nil {
		t.Fatalf("expected daily workspace memory file to exist: %v", err)
	}
}

func TestNewTargetAppResolvesImplicitMainAgentProfile(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"
	cfg.Agent.Name = "Go Expert"
	cfg.Agent.ActiveProfile = ""
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workflows", "personal")
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")

	enabled := config.BoolPtr(true)
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "qwen",
			Name:         "Qwen",
			Provider:     "qwen",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:       "provider-key",
			DefaultModel: "qwen-max",
			Enabled:      enabled,
		},
	}
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:            "Go Expert",
			Description:     "Go specialist",
			WorkingDir:      filepath.Join(tempDir, "workflows", "go-expert"),
			PermissionLevel: "limited",
			ProviderRef:     "qwen",
			DefaultModel:    "qwen-max",
			Enabled:         enabled,
		},
	}

	configPath := filepath.Join(tempDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.MkdirAll(cfg.Skills.Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", cfg.Skills.Dir, err)
	}
	if err := os.MkdirAll(cfg.Plugins.Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", cfg.Plugins.Dir, err)
	}

	app, err := NewTargetApp(configPath, "", "")
	if err != nil {
		t.Fatalf("NewTargetApp: %v", err)
	}
	t.Cleanup(func() { app.Memory.Close() })

	absProfileWorkingDir, err := filepath.Abs(filepath.Join(tempDir, "workflows", "go-expert"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	if app.Config.Agent.ActiveProfile != "Go Expert" {
		t.Fatalf("expected active profile Go Expert, got %q", app.Config.Agent.ActiveProfile)
	}
	if app.Config.Agent.Name != "Go Expert" {
		t.Fatalf("expected agent name Go Expert, got %q", app.Config.Agent.Name)
	}
	if app.Config.LLM.Provider != "qwen" {
		t.Fatalf("expected provider qwen, got %q", app.Config.LLM.Provider)
	}
	if app.Config.LLM.Model != "qwen-max" {
		t.Fatalf("expected model qwen-max, got %q", app.Config.LLM.Model)
	}
	if app.Config.LLM.APIKey != "provider-key" {
		t.Fatalf("expected provider API key to override global key")
	}
	if app.WorkingDir != absProfileWorkingDir {
		t.Fatalf("expected working dir %q, got %q", absProfileWorkingDir, app.WorkingDir)
	}
}

func TestNewTargetAppAutoCompletesBootstrapWhenAgentProfileAlreadyConfigured(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.Name = "binbin"
	cfg.Agent.Description = "Execution helper"
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workflows", "default")
	cfg.Agent.Lang = "zh-CN"
	cfg.Agent.WorkFocus = "本地编码与项目维护"
	cfg.Agent.BehaviorStyle = "简洁、主动、中文优先"
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")

	configPath := filepath.Join(tempDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	for _, dir := range []string{cfg.Skills.Dir, cfg.Plugins.Dir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}

	app, err := NewTargetApp(configPath, "", "")
	if err != nil {
		t.Fatalf("NewTargetApp: %v", err)
	}
	t.Cleanup(func() { app.Memory.Close() })

	if _, err := os.Stat(filepath.Join(app.WorkingDir, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected BOOTSTRAP.md to be removed for configured workspace, stat err=%v", err)
	}

	userData, err := os.ReadFile(filepath.Join(app.WorkingDir, "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md): %v", err)
	}
	if !strings.Contains(string(userData), "Default language: zh-CN") {
		t.Fatalf("expected USER.md to include configured language, got %q", string(userData))
	}

	identityData, err := os.ReadFile(filepath.Join(app.WorkingDir, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("ReadFile(IDENTITY.md): %v", err)
	}
	identityText := string(identityData)
	if !strings.Contains(identityText, "本地编码与项目维护") {
		t.Fatalf("expected IDENTITY.md to include configured work focus, got %q", identityText)
	}
	if !strings.Contains(identityText, "简洁、主动、中文优先") {
		t.Fatalf("expected IDENTITY.md to include configured behavior style, got %q", identityText)
	}
}

func filepathModTimeFixture() (ts time.Time) {
	return time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
}
