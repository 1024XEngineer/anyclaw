package task

import (
	"errors"
	"strings"
	"testing"
)

func TestPlanSpecLifecycleAndSerialization(t *testing.T) {
	spec := NewPlanSpec("task-1", "Plan Title", "Plan Description")
	if spec.ID == "" || spec.Mode != "planner" || spec.RiskLevel != "medium" || !spec.RequiresApproval {
		t.Fatalf("unexpected defaults: %+v", spec)
	}

	spec.AddWorkflowStep(WorkflowDescriptor{
		Name:        "sync",
		Description: "sync data",
		Plugin:      "plugin-x",
		Action:      "run",
		ToolName:    "tool-x",
		Pairing:     "reviewer",
	}, map[string]any{"target": "demo"})
	spec.AddToolStep("sudo rm", map[string]any{"path": "/tmp/demo"}, true)
	spec.AddVerificationStep(1, VerificationSpec{
		Type:       "file-exists",
		Parameters: map[string]any{"path": "/tmp/demo"},
	})
	spec.AddRollbackStep(2, RollbackSpec{
		OnFailure: true,
		Steps: []RollbackStep{{
			Action: "restore",
			Inputs: map[string]any{"path": "/tmp/demo"},
		}},
	})

	if len(spec.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(spec.Steps))
	}
	if spec.Steps[0].Index != 1 || !strings.Contains(spec.Steps[0].Description, "paired: reviewer") {
		t.Fatalf("unexpected workflow step: %+v", spec.Steps[0])
	}
	if spec.Steps[1].RiskLevel != "high" {
		t.Fatalf("expected high-risk tool step, got %+v", spec.Steps[1])
	}

	if err := spec.MarkStepStarted(1); err != nil {
		t.Fatalf("MarkStepStarted: %v", err)
	}
	if spec.Steps[0].Status != "running" || spec.Steps[0].StartedAt == nil {
		t.Fatalf("expected running step after start, got %+v", spec.Steps[0])
	}
	if err := spec.MarkStepCompleted(1, map[string]any{"ok": true}); err != nil {
		t.Fatalf("MarkStepCompleted: %v", err)
	}
	if err := spec.MarkStepFailed(2, errors.New("boom")); err != nil {
		t.Fatalf("MarkStepFailed: %v", err)
	}
	if spec.Steps[0].Status != "completed" || spec.Steps[1].Status != "failed" {
		t.Fatalf("unexpected step statuses: %+v", spec.Steps[:2])
	}

	spec.AddEvidence(2, "log", "failure", map[string]any{"code": 500})
	spec.AddArtifact(1, "report", "file", "/tmp/report.txt", map[string]any{"size": 123})
	spec.AddRecoveryPoint(2, "checkpoint", map[string]any{"cursor": 2})

	if len(spec.Evidence) != 1 || len(spec.Artifacts) != 1 || len(spec.RecoveryPoints) != 1 {
		t.Fatalf("expected evidence/artifact/recovery point to be recorded: %+v", spec)
	}
	if current := spec.GetCurrentStep(); current == nil || current.Index != 3 {
		t.Fatalf("expected current step 3, got %+v", current)
	}
	if len(spec.GetCompletedSteps()) != 1 {
		t.Fatalf("expected one completed step, got %+v", spec.GetCompletedSteps())
	}
	if len(spec.GetFailedSteps()) != 1 {
		t.Fatalf("expected one failed step, got %+v", spec.GetFailedSteps())
	}

	data, err := spec.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	decoded, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if decoded.Title != spec.Title || len(decoded.Steps) != len(spec.Steps) {
		t.Fatalf("unexpected decoded plan: %+v", decoded)
	}

	if err := spec.MarkStepStarted(99); err == nil {
		t.Fatal("expected out-of-range step start to fail")
	}
	if _, err := FromJSON([]byte("{")); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestPlanBuilderAndAssessToolRisk(t *testing.T) {
	builder := NewPlanBuilder("task-2", "Builder Plan", "Built")
	spec := builder.
		SetMode("workflow").
		SetRiskLevel("low").
		AddRiskLabel("network").
		SetPrivacyScope("work").
		SetDataScope("write").
		SetRequiresApproval(false).
		SetApprovalScope("tool").
		AddWorkflowStep(WorkflowDescriptor{Name: "wf", Description: "workflow"}, map[string]any{"x": 1}).
		AddToolStep("run_command", map[string]any{"cmd": "echo hi"}, true).
		AddVerificationStep(1, VerificationSpec{Type: "text-contains", Parameters: map[string]any{"text": "ok"}}).
		AddRollbackStep(2, RollbackSpec{OnCancel: true}).
		Build()

	if spec.Mode != "workflow" || spec.RiskLevel != "low" || spec.PrivacyScope != "work" || spec.DataScope != "write" {
		t.Fatalf("unexpected builder config: %+v", spec)
	}
	if len(spec.RiskLabels) != 1 || spec.RiskLabels[0] != "network" {
		t.Fatalf("unexpected risk labels: %+v", spec.RiskLabels)
	}
	if len(spec.Steps) != 4 {
		t.Fatalf("expected builder to add 4 steps, got %d", len(spec.Steps))
	}

	if got := assessToolRisk("sudo rm -rf"); got != "high" {
		t.Fatalf("expected high risk, got %q", got)
	}
	if got := assessToolRisk("run_command"); got != "medium" {
		t.Fatalf("expected medium risk, got %q", got)
	}
	if got := assessToolRisk("read_file"); got != "low" {
		t.Fatalf("expected low risk, got %q", got)
	}
}
