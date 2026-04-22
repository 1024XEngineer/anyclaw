package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestResolveRuntimePathsResolvesControlUIRoot(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "anyclaw.json")

	cfg := config.DefaultConfig()
	cfg.Gateway.ControlUI.Root = "dist/control-ui"

	ResolveRuntimePaths(cfg, configPath)

	expected := filepath.Join(configDir, "dist", "control-ui")
	if cfg.Gateway.ControlUI.Root != expected {
		t.Fatalf("expected control UI root %q, got %q", expected, cfg.Gateway.ControlUI.Root)
	}
}

func TestResolveRuntimePathsResolvesMultipleRuntimeFields(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "anyclaw.json")

	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = "workspace"
	cfg.Agent.WorkingDir = "working"
	cfg.Agent.Profiles = []config.AgentProfile{
		{Name: "main", WorkingDir: "profiles/main"},
	}
	cfg.Orchestrator.SubAgents = []config.SubAgentConfig{
		{Name: "sub", WorkingDir: "subagents/worker"},
	}
	cfg.Skills.Dir = "skills"
	cfg.Plugins.Dir = "plugins"
	cfg.Memory.Dir = "memory"
	cfg.Security.AuditLog = "logs/audit.log"
	cfg.Sandbox.BaseDir = "sandbox"
	cfg.Daemon.PIDFile = "run/anyclaw.pid"
	cfg.Daemon.LogFile = "logs/daemon.log"

	ResolveRuntimePaths(cfg, configPath)

	assertResolved := func(name string, got string, parts ...string) {
		t.Helper()
		want := filepath.Join(append([]string{configDir}, parts...)...)
		if got != want {
			t.Fatalf("expected %s %q, got %q", name, want, got)
		}
	}

	assertResolved("agent.work_dir", cfg.Agent.WorkDir, "workspace")
	assertResolved("agent.working_dir", cfg.Agent.WorkingDir, "working")
	assertResolved("agent.profile.working_dir", cfg.Agent.Profiles[0].WorkingDir, "profiles", "main")
	assertResolved("orchestrator.sub_agent.working_dir", cfg.Orchestrator.SubAgents[0].WorkingDir, "subagents", "worker")
	assertResolved("skills.dir", cfg.Skills.Dir, "skills")
	assertResolved("plugins.dir", cfg.Plugins.Dir, "plugins")
	assertResolved("memory.dir", cfg.Memory.Dir, "memory")
	assertResolved("security.audit_log", cfg.Security.AuditLog, "logs", "audit.log")
	assertResolved("sandbox.base_dir", cfg.Sandbox.BaseDir, "sandbox")
	assertResolved("daemon.pid_file", cfg.Daemon.PIDFile, "run", "anyclaw.pid")
	assertResolved("daemon.log_file", cfg.Daemon.LogFile, "logs", "daemon.log")
}

func TestResolveConfigPathDefaultsAndPreservesAbsolutePath(t *testing.T) {
	if got := ResolveConfigPath(""); !filepath.IsAbs(got) || filepath.Base(got) != "anyclaw.json" {
		t.Fatalf("expected default absolute anyclaw.json path, got %q", got)
	}

	abs := filepath.Join(t.TempDir(), "config.json")
	if got := ResolveConfigPath(abs); got != abs {
		t.Fatalf("expected absolute path to be preserved, got %q", got)
	}
}
