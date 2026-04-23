package state

import (
	"strings"
	"testing"
	"time"
)

func TestSessionExecutionExportedHelpers(t *testing.T) {
	tests := []struct {
		name          string
		session       *Session
		wantBinding   SessionExecutionBinding
		wantAgent     string
		wantWorkspace string
	}{
		{
			name: "top-level fields only",
			session: &Session{
				Agent:     "agent-top",
				Org:       "org-top",
				Project:   "project-top",
				Workspace: "workspace-top",
			},
			wantBinding: SessionExecutionBinding{
				Agent:     "agent-top",
				Org:       "org-top",
				Project:   "project-top",
				Workspace: "workspace-top",
			},
			wantAgent:     "agent-top",
			wantWorkspace: "workspace-top",
		},
		{
			name: "binding fields only",
			session: &Session{
				ExecutionBinding: SessionExecutionBinding{
					Agent:     "agent-bound",
					Org:       "org-bound",
					Project:   "project-bound",
					Workspace: "workspace-bound",
				},
			},
			wantBinding: SessionExecutionBinding{
				Agent:     "agent-bound",
				Org:       "org-bound",
				Project:   "project-bound",
				Workspace: "workspace-bound",
			},
			wantAgent:     "agent-bound",
			wantWorkspace: "workspace-bound",
		},
		{
			name: "binding takes precedence when both differ",
			session: &Session{
				Agent:     "agent-top",
				Org:       "org-top",
				Project:   "project-top",
				Workspace: "workspace-top",
				ExecutionBinding: SessionExecutionBinding{
					Agent:     "agent-bound",
					Org:       "org-bound",
					Project:   "project-bound",
					Workspace: "workspace-bound",
				},
			},
			wantBinding: SessionExecutionBinding{
				Agent:     "agent-bound",
				Org:       "org-bound",
				Project:   "project-bound",
				Workspace: "workspace-bound",
			},
			wantAgent:     "agent-bound",
			wantWorkspace: "workspace-bound",
		},
		{
			name:          "nil session",
			session:       nil,
			wantBinding:   SessionExecutionBinding{},
			wantAgent:     "",
			wantWorkspace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBinding := SessionExecutionBindingValue(tt.session)
			if gotBinding != tt.wantBinding {
				t.Fatalf("expected binding %#v, got %#v", tt.wantBinding, gotBinding)
			}
			if got := SessionExecutionAgent(tt.session); got != tt.wantAgent {
				t.Fatalf("expected agent %q, got %q", tt.wantAgent, got)
			}
			if got := SessionExecutionWorkspace(tt.session); got != tt.wantWorkspace {
				t.Fatalf("expected workspace %q, got %q", tt.wantWorkspace, got)
			}
			agent, org, project, workspace := SessionExecutionTarget(tt.session)
			if agent != tt.wantBinding.Agent || org != tt.wantBinding.Org || project != tt.wantBinding.Project || workspace != tt.wantBinding.Workspace {
				t.Fatalf("expected target (%q, %q, %q, %q), got (%q, %q, %q, %q)", tt.wantBinding.Agent, tt.wantBinding.Org, tt.wantBinding.Project, tt.wantBinding.Workspace, agent, org, project, workspace)
			}
			org, project, workspace = SessionExecutionHierarchy(tt.session)
			if org != tt.wantBinding.Org || project != tt.wantBinding.Project || workspace != tt.wantBinding.Workspace {
				t.Fatalf("expected hierarchy (%q, %q, %q), got (%q, %q, %q)", tt.wantBinding.Org, tt.wantBinding.Project, tt.wantBinding.Workspace, org, project, workspace)
			}
		})
	}
}

func TestExportedCloneHelpersReturnIsolatedCopies(t *testing.T) {
	event := &Event{ID: "evt-1", Payload: map[string]any{"value": 1}}
	eventClone := CloneEvent(event)
	eventClone.Payload["value"] = 2
	if event.Payload["value"] != 1 {
		t.Fatalf("expected CloneEvent to isolate payload, got %#v", event.Payload)
	}

	approval := &Approval{ID: "approval-1", Payload: map[string]any{"value": 1}}
	approvalClone := CloneApproval(approval)
	approvalClone.Payload["value"] = 2
	if approval.Payload["value"] != 1 {
		t.Fatalf("expected CloneApproval to isolate payload, got %#v", approval.Payload)
	}

	point := &TaskRecoveryPoint{Kind: "desktop", Data: map[string]any{"value": 1}}
	pointClone := CloneTaskRecoveryPoint(point)
	pointClone.Data["value"] = 2
	if point.Data["value"] != 1 {
		t.Fatalf("expected CloneTaskRecoveryPoint to isolate data, got %#v", point.Data)
	}
}

func TestNormalizeParticipantsShortenTitleAndUniqueID(t *testing.T) {
	participants := NormalizeParticipants(" MainAgent ", []string{"", "Reviewer", "MainAgent", " Reviewer ", "Observer"})
	wantParticipants := []string{"MainAgent", "Reviewer", "Observer"}
	if len(participants) != len(wantParticipants) {
		t.Fatalf("expected normalized participants %#v, got %#v", wantParticipants, participants)
	}
	for i := range wantParticipants {
		if participants[i] != wantParticipants[i] {
			t.Fatalf("expected normalized participants %#v, got %#v", wantParticipants, participants)
		}
	}

	if got := ShortenTitle(""); got != "New session" {
		t.Fatalf("expected empty title to fall back, got %q", got)
	}
	long := strings.Repeat("a", 60)
	if got := ShortenTitle(long); got != strings.Repeat("a", 48) {
		t.Fatalf("expected long title to be truncated, got %q", got)
	}

	first := UniqueID("state")
	second := UniqueID("state")
	if !strings.HasPrefix(first, "state_") || !strings.HasPrefix(second, "state_") {
		t.Fatalf("expected prefixed IDs, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("expected unique IDs, got duplicate %q", first)
	}
}

func TestExportedCloneHelpersHandleNil(t *testing.T) {
	if CloneEvent(nil) != nil {
		t.Fatal("expected CloneEvent(nil) to return nil")
	}
	if CloneApproval(nil) != nil {
		t.Fatal("expected CloneApproval(nil) to return nil")
	}
	if CloneTaskRecoveryPoint(nil) != nil {
		t.Fatal("expected CloneTaskRecoveryPoint(nil) to return nil")
	}

	if SessionExecutionAgent(nil) != "" {
		t.Fatal("expected nil session agent to be empty")
	}
	if SessionExecutionWorkspace(nil) != "" {
		t.Fatal("expected nil session workspace to be empty")
	}
}

func TestCloneDesktopPlanExecutionStateIsolated(t *testing.T) {
	state := &DesktopPlanExecutionState{
		Steps: []DesktopPlanStepExecutionState{
			{Index: 1, Status: "running", UpdatedAt: time.Now().UTC().Format(time.RFC3339)},
		},
	}
	clone := CloneDesktopPlanExecutionState(state)
	clone.Steps[0].Status = "done"
	if state.Steps[0].Status != "running" {
		t.Fatalf("expected cloned desktop plan steps to be isolated, got %#v", state.Steps)
	}
}
