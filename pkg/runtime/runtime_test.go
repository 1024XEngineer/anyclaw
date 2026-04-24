package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestConfiguredAgentSkillNamesFallsBackToMainAgentSkills(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.Name = "Personal Assistant"
	cfg.Agent.Skills = []config.AgentSkillRef{
		{Name: "vision-agent", Enabled: true},
		{Name: "vision-agent", Enabled: true},
		{Name: "disabled-skill", Enabled: false},
	}
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:    "Go Expert",
			Enabled: config.BoolPtr(true),
			Skills: []config.AgentSkillRef{
				{Name: "coder", Enabled: true},
			},
		},
	}

	got := configuredAgentSkillNames(cfg)
	if len(got) != 1 || got[0] != "vision-agent" {
		t.Fatalf("expected only main-agent skill vision-agent, got %#v", got)
	}
}

func TestResolveMainAgentPersonalityDoesNotFallbackToFirstEnabledProfile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.Name = "Personal Assistant"
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:        "Go Expert",
			Enabled:     config.BoolPtr(true),
			Personality: config.PersonalitySpec{Tone: "严谨", Traits: []string{"精确"}},
		},
	}

	got := resolveMainAgentPersonality(cfg)
	if !reflect.DeepEqual(got, config.PersonalitySpec{}) {
		t.Fatalf("expected zero personality when no profile matches main agent, got %#v", got)
	}
}

func TestBootstrapLoadsMainAgentSkillsWhenNoProfileMatches(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.Name = "Personal Assistant"
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workflows", "personal")
	cfg.LLM.APIKey = "test-key"
	cfg.Agent.Skills = []config.AgentSkillRef{
		{Name: "vision-agent", Enabled: true},
	}
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:    "Go Expert",
			Enabled: config.BoolPtr(true),
			Skills: []config.AgentSkillRef{
				{Name: "coder", Enabled: true},
			},
		},
	}
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")

	if err := os.MkdirAll(cfg.Plugins.Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll plugins dir: %v", err)
	}
	writeTestSkill(t, cfg.Skills.Dir, "coder")
	writeTestSkill(t, cfg.Skills.Dir, "vision-agent")

	app, err := Bootstrap(BootstrapOptions{
		ConfigPath: filepath.Join(tempDir, "anyclaw.json"),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	skills := app.Agent.ListSkills()
	if len(skills) != 1 {
		t.Fatalf("expected exactly 1 loaded skill, got %#v", skills)
	}
	if skills[0].Name != "vision-agent" {
		t.Fatalf("expected vision-agent to be loaded, got %#v", skills)
	}
}

func TestBootstrapResolvesRelativePathsFromConfigPath(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	configPath := filepath.Join(configDir, "anyclaw.json")

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Model = "qwen2.5"
	cfg.Agent.WorkDir = ".anyclaw"
	cfg.Agent.WorkingDir = "workspace"
	cfg.Skills.Dir = "skills"
	cfg.Plugins.Dir = "plugins"
	cfg.Security.AuditLog = ".anyclaw/audit/audit.jsonl"

	if err := os.MkdirAll(filepath.Join(configDir, "plugins"), 0o755); err != nil {
		t.Fatalf("MkdirAll plugins dir: %v", err)
	}
	writeTestSkill(t, filepath.Join(configDir, "skills"), "coder")

	app, err := Bootstrap(BootstrapOptions{
		ConfigPath: configPath,
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	wantWorkDir := filepath.Join(configDir, ".anyclaw")
	if app.WorkDir != wantWorkDir {
		t.Fatalf("expected work dir %q, got %q", wantWorkDir, app.WorkDir)
	}

	wantWorkingDir := filepath.Join(configDir, "workspace")
	if app.WorkingDir != wantWorkingDir {
		t.Fatalf("expected working dir %q, got %q", wantWorkingDir, app.WorkingDir)
	}

	wantSkillsDir := filepath.Join(configDir, "skills")
	if app.Config.Skills.Dir != wantSkillsDir {
		t.Fatalf("expected skills dir %q, got %q", wantSkillsDir, app.Config.Skills.Dir)
	}

	wantPluginsDir := filepath.Join(configDir, "plugins")
	if app.Config.Plugins.Dir != wantPluginsDir {
		t.Fatalf("expected plugins dir %q, got %q", wantPluginsDir, app.Config.Plugins.Dir)
	}

	wantAuditLog := filepath.Join(configDir, ".anyclaw", "audit", "audit.jsonl")
	if app.Config.Security.AuditLog != wantAuditLog {
		t.Fatalf("expected audit log %q, got %q", wantAuditLog, app.Config.Security.AuditLog)
	}
}

func writeTestSkill(t *testing.T, skillsDir string, name string) {
	t.Helper()
	skillDir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	payload := map[string]any{
		"name":        name,
		"description": name + " test skill",
		"version":     "1.0.0",
		"prompts": map[string]string{
			"system": "test prompt",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal skill payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile skill.json: %v", err)
	}
}
