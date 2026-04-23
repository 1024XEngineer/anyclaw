package desktop

import (
	"context"
	"testing"
	"time"
)

func TestWithDesktopPlanStateHookAndReport(t *testing.T) {
	base := context.Background()
	state := DesktopPlanExecutionState{
		ToolName: "desktop",
		Status:   "running",
		Steps: []DesktopPlanStepExecutionState{
			{Index: 0, Tool: "click", Status: "done"},
		},
	}

	var called bool
	var got DesktopPlanExecutionState
	ctx := WithDesktopPlanStateHook(base, func(_ context.Context, s DesktopPlanExecutionState) {
		called = true
		got = s
	})

	ReportDesktopPlanState(ctx, state)
	if !called {
		t.Fatal("expected hook to be called")
	}
	if got.ToolName != state.ToolName || got.Status != state.Status {
		t.Fatalf("unexpected reported state: %+v", got)
	}

	ReportDesktopPlanState(nil, state)
	ReportDesktopPlanState(base, state)
}

func TestWithDesktopPlanResumeStateRoundTripUsesClone(t *testing.T) {
	base := context.Background()
	original := &DesktopPlanExecutionState{
		ToolName: "desktop",
		Steps: []DesktopPlanStepExecutionState{
			{Index: 0, Tool: "type", Status: "running"},
		},
	}

	ctx := WithDesktopPlanResumeState(base, original)
	original.ToolName = "mutated"
	original.Steps[0].Tool = "changed"

	resumed := DesktopPlanResumeStateFromContext(ctx)
	if resumed == nil {
		t.Fatal("expected resume state from context")
	}
	if resumed.ToolName != "desktop" {
		t.Fatalf("expected cloned tool name, got %q", resumed.ToolName)
	}
	if resumed.Steps[0].Tool != "type" {
		t.Fatalf("expected cloned step tool, got %q", resumed.Steps[0].Tool)
	}

	resumed.ToolName = "again"
	resumed.Steps[0].Tool = "again"
	check := DesktopPlanResumeStateFromContext(ctx)
	if check.ToolName != "desktop" || check.Steps[0].Tool != "type" {
		t.Fatalf("expected context state to stay isolated, got %+v", check)
	}

	if got := DesktopPlanResumeStateFromContext(nil); got != nil {
		t.Fatalf("expected nil context lookup to return nil, got %+v", got)
	}
}

func TestCloneDesktopPlanExecutionStateAndTimestamp(t *testing.T) {
	if CloneDesktopPlanExecutionState(nil) != nil {
		t.Fatal("expected nil clone to stay nil")
	}

	state := &DesktopPlanExecutionState{
		ToolName: "desktop",
		Steps: []DesktopPlanStepExecutionState{
			{Index: 1, Tool: "submit", Status: "done"},
		},
	}
	cloned := CloneDesktopPlanExecutionState(state)
	if cloned == nil {
		t.Fatal("expected cloned state")
	}

	state.Steps[0].Tool = "mutated"
	if cloned.Steps[0].Tool != "submit" {
		t.Fatalf("expected steps to be copied, got %+v", cloned.Steps)
	}

	ts := TimestampNowRFC3339()
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Fatalf("expected RFC3339 timestamp, got %q: %v", ts, err)
	}
	if parsed.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got %s", parsed.Location())
	}
}
