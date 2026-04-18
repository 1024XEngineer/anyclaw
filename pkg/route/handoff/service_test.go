package handoff

import "testing"

type stubMatcher struct {
	names []string
}

func (s stubMatcher) AvailableAgentNames() []string {
	return s.names
}

func TestPrepareNormalizesHandoffEntry(t *testing.T) {
	router := NewRouter(nil)

	req := router.Prepare(HandoffRoutingEntry{
		SessionID:           " session-1 ",
		UserInput:           " inspect repository ",
		PreferredSubagentID: " specialist-a ",
		SkipDelegation:      true,
	})

	if req.SessionID != "session-1" {
		t.Fatalf("expected trimmed session id, got %q", req.SessionID)
	}
	if req.UserInput != "inspect repository" {
		t.Fatalf("expected trimmed user input, got %q", req.UserInput)
	}
	if req.PreferredSubagentID != "specialist-a" {
		t.Fatalf("expected trimmed preferred agent, got %q", req.PreferredSubagentID)
	}
	if !req.SkipDelegation {
		t.Fatal("expected skip delegation flag to be preserved")
	}
}

func TestPlanReturnsMainWhenSkipDelegationIsRequested(t *testing.T) {
	router := NewRouter(stubMatcher{names: []string{"specialist-a"}})

	plan := router.Plan(HandoffRequest{
		SessionID:      "session-1",
		UserInput:      "Inspect the repository",
		SkipDelegation: true,
	}, PlanOptions{PersistentFirst: true})

	if plan.Mode != ModeMain {
		t.Fatalf("expected main mode, got %q", plan.Mode)
	}
	if plan.TargetAgentID != "" {
		t.Fatalf("expected no target agent, got %q", plan.TargetAgentID)
	}
}

func TestPlanUsesPreferredPersistentSubagentWhenAvailable(t *testing.T) {
	router := NewRouter(stubMatcher{names: []string{"specialist-a", "specialist-b"}})

	plan := router.Plan(HandoffRequest{
		SessionID:           "session-1",
		UserInput:           "Inspect the repository",
		PreferredSubagentID: "SPECIALIST-B",
	}, PlanOptions{PersistentFirst: true})

	if plan.Mode != ModePersistentSubagent {
		t.Fatalf("expected persistent subagent mode, got %q", plan.Mode)
	}
	if plan.TargetAgentID != "specialist-b" {
		t.Fatalf("expected resolved target agent, got %q", plan.TargetAgentID)
	}
}

func TestPlanFallsBackToTemporaryWhenPreferredPersistentSubagentIsUnavailable(t *testing.T) {
	router := NewRouter(stubMatcher{names: []string{"specialist-a"}})

	plan := router.Plan(HandoffRequest{
		SessionID:           "session-1",
		UserInput:           "Inspect the repository",
		PreferredSubagentID: "specialist-b",
	}, PlanOptions{PersistentFirst: true, AllowTemporary: true})

	if plan.Mode != ModeTemporarySubagent {
		t.Fatalf("expected temporary subagent mode, got %q", plan.Mode)
	}
	if plan.TargetAgentID != "specialist-b" {
		t.Fatalf("expected temporary target to keep requested id, got %q", plan.TargetAgentID)
	}
}

func TestPlanUsesSingleAvailablePersistentSubagentWhenUnambiguous(t *testing.T) {
	router := NewRouter(stubMatcher{names: []string{"specialist-a"}})

	plan := router.Plan(HandoffRequest{
		SessionID: "session-1",
		UserInput: "Inspect the repository",
	}, PlanOptions{PersistentFirst: true})

	if plan.Mode != ModePersistentSubagent {
		t.Fatalf("expected persistent subagent mode, got %q", plan.Mode)
	}
	if plan.TargetAgentID != "specialist-a" {
		t.Fatalf("expected the single available specialist to be selected, got %q", plan.TargetAgentID)
	}
}

func TestPlanKeepsTaskOnMainWhenMultiplePersistentSubagentsAreAvailableWithoutPreference(t *testing.T) {
	router := NewRouter(stubMatcher{names: []string{"specialist-a", "specialist-b"}})

	plan := router.Plan(HandoffRequest{
		SessionID: "session-1",
		UserInput: "Inspect the repository",
	}, PlanOptions{PersistentFirst: true, AllowTemporary: true})

	if plan.Mode != ModeMain {
		t.Fatalf("expected main mode, got %q", plan.Mode)
	}
	if plan.TargetAgentID != "" {
		t.Fatalf("expected no target agent, got %q", plan.TargetAgentID)
	}
}
