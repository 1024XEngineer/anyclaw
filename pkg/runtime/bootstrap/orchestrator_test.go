package bootstrap

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestBuildOrchestratorConfigClampsProfileAgentPermissionToMainAgent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.PermissionLevel = "limited"
	cfg.Orchestrator.AgentNames = []string{"worker"}
	cfg.Agent.Profiles = []config.AgentProfile{{
		Name:            "worker",
		Enabled:         config.BoolPtr(true),
		PermissionLevel: "full",
	}}

	orch := BuildOrchestratorConfig(cfg, t.TempDir(), t.TempDir())
	if len(orch.AgentDefinitions) != 1 {
		t.Fatalf("expected 1 agent definition, got %#v", orch.AgentDefinitions)
	}
	if orch.AgentDefinitions[0].PermissionLevel != "limited" {
		t.Fatalf("expected clamped permission limited, got %q", orch.AgentDefinitions[0].PermissionLevel)
	}
}

func TestBuildOrchestratorConfigClampsExplicitSubAgentPermissionToMainAgent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.PermissionLevel = "read-only"
	cfg.Orchestrator.SubAgents = []config.SubAgentConfig{{
		Name:            "worker",
		PermissionLevel: "full",
	}}

	orch := BuildOrchestratorConfig(cfg, t.TempDir(), t.TempDir())
	if len(orch.AgentDefinitions) != 1 {
		t.Fatalf("expected 1 agent definition, got %#v", orch.AgentDefinitions)
	}
	if orch.AgentDefinitions[0].PermissionLevel != "read-only" {
		t.Fatalf("expected clamped permission read-only, got %q", orch.AgentDefinitions[0].PermissionLevel)
	}
}

func TestBuildOrchestratorConfigTreatsBlankSubAgentPermissionAsLimitedBeforeClamp(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.PermissionLevel = "read-only"
	cfg.Orchestrator.SubAgents = []config.SubAgentConfig{{
		Name: "worker",
	}}

	orch := BuildOrchestratorConfig(cfg, t.TempDir(), t.TempDir())
	if len(orch.AgentDefinitions) != 1 {
		t.Fatalf("expected 1 agent definition, got %#v", orch.AgentDefinitions)
	}
	if orch.AgentDefinitions[0].PermissionLevel != "read-only" {
		t.Fatalf("expected blank subagent permission to clamp to read-only, got %q", orch.AgentDefinitions[0].PermissionLevel)
	}
}
