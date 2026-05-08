package agent

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/clihub"
)

func TestCapabilityPlannerSimpleRequestUsesMainAgent(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{UserInput: "hello"})
	if plan.TaskClass != CapabilityTaskSimple || plan.Route != CapabilityRouteMainAgent || plan.ShouldExposeMarketSearch {
		t.Fatalf("plan = %#v, want simple main agent without market", plan)
	}
}

func TestCapabilityPlannerPrefersLocalSkill(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "write release notes for this changelog",
		Skills: []skills.SkillCatalogEntry{{
			Name:        "release-notes",
			Description: "Draft release notes and changelog summaries.",
			Category:    "writing",
			Installed:   true,
		}},
	})
	if plan.Route != CapabilityRouteUseSkill || plan.KindHint != "skill" || len(plan.LocalMatches) != 1 {
		t.Fatalf("plan = %#v, want local skill route", plan)
	}
	if plan.ShouldExposeMarketSearch {
		t.Fatalf("local skill should avoid market search: %#v", plan)
	}
}

func TestCapabilityPlannerPrefersLocalAgent(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "review this pull request for code risks",
		Agents: []CapabilityAgentSummary{{
			Name:        "code-reviewer",
			Description: "Reviews pull requests and code risks.",
			Domain:      "code review",
			Expertise:   []string{"pull request", "security review"},
		}},
	})
	if plan.Route != CapabilityRouteDelegateAgent || plan.KindHint != "agent" || len(plan.LocalMatches) != 1 {
		t.Fatalf("plan = %#v, want local agent delegation", plan)
	}
}

func TestCapabilityPlannerPrefersCLIHubCapability(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "render the shotcut video timeline",
		CLICapabilities: []clihub.Capability{{
			Harness:  "shotcut",
			Group:    "Timeline",
			Command:  "render",
			Action:   "Timeline render",
			Category: "video",
			Keywords: []string{"video", "timeline", "render", "shotcut"},
		}},
	})
	if plan.Route != CapabilityRouteUseCLI || plan.KindHint != "cli" || len(plan.LocalMatches) != 1 {
		t.Fatalf("plan = %#v, want CLIHub route", plan)
	}
}

func TestCapabilityPlannerUsesInstalledLocalMarketplaceArtifact(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "diagnose repository health",
		LocalArtifacts: []CapabilityArtifactSummary{{
			ArtifactID:   "cloud.cli.repo-health",
			Kind:         "cli",
			Name:         "Repo Health",
			Description:  "Diagnose repository health.",
			Capabilities: []string{"repo health", "diagnose repository"},
			Installed:    true,
			Bound:        true,
		}},
	})
	if plan.Route != CapabilityRouteUseCLI || plan.KindHint != "cli" || len(plan.LocalMatches) != 1 {
		t.Fatalf("plan = %#v, want installed local marketplace cli route", plan)
	}
}

func TestCapabilityPlannerSpecializedGapExposesMarketSearch(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "review this pull request for security and maintainability",
	})
	if plan.Route != CapabilityRouteSearchMarket || !plan.ShouldExposeMarketSearch || plan.KindHint != "agent" {
		t.Fatalf("plan = %#v, want market search for missing specialized agent", plan)
	}
}

func TestCapabilityPlannerLocalExecutionDoesNotExposeMarketSearch(t *testing.T) {
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "read the README and run go test",
		Tools: []tools.ToolInfo{
			{Name: "read_file"},
			{Name: "run_command"},
		},
	})
	if plan.Route != CapabilityRouteMainAgent || plan.TaskClass != CapabilityTaskLocalExecution {
		t.Fatalf("plan = %#v, want local execution main agent route", plan)
	}
	if plan.ShouldExposeMarketSearch {
		t.Fatalf("local execution should not expose market search: %#v", plan)
	}
}
