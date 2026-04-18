package handoff

import "testing"

type stubCatalog struct {
	names []string
}

func (s stubCatalog) AvailableAgentNames() []string {
	return s.names
}

func TestBuildPlanRequiresExplicitTargetsWhenMultipleSpecialistsExist(t *testing.T) {
	service := NewService(stubCatalog{names: []string{"specialist-a", "specialist-b"}})

	if _, err := service.BuildPlan(Request{
		Task:            "Inspect the repository",
		SuccessCriteria: "Return a concise summary",
	}); err == nil {
		t.Fatal("expected missing explicit targets to fail when multiple specialists are available")
	}
}

func TestBuildPlanRejectsMissingTask(t *testing.T) {
	service := NewService(stubCatalog{names: []string{"specialist-a"}})

	if _, err := service.BuildPlan(Request{}); err == nil {
		t.Fatal("expected missing task to fail")
	}
}

func TestBuildPlanRejectsUnknownExplicitTargets(t *testing.T) {
	service := NewService(stubCatalog{names: []string{"specialist-a"}})

	if _, err := service.BuildPlan(Request{
		Task:       "Inspect the repository",
		AgentNames: []string{"specialist-b"},
	}); err == nil {
		t.Fatal("expected unknown explicit agent to fail")
	}
}

func TestBuildPlanUsesSingleAvailableSpecialistWhenUnambiguous(t *testing.T) {
	service := NewService(stubCatalog{names: []string{"specialist-a"}})

	plan, err := service.BuildPlan(Request{
		Task:            "Inspect the repository",
		SuccessCriteria: "Return a concise summary",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.TargetAgents) != 1 || plan.TargetAgents[0] != "specialist-a" {
		t.Fatalf("expected the single available specialist to be selected, got %#v", plan.TargetAgents)
	}
	if plan.Brief == "" {
		t.Fatal("expected handoff brief to be generated")
	}
}
