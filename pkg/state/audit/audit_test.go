package audit

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerAppendTailAndLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger := New(path, "agent-1")

	approval := &Approval{
		ID:        "ap-1",
		TaskID:    "task-1",
		SessionID: "session-1",
		UserID:    "user-1",
		Scope:     "tool",
		Category:  "security",
		Action:    "run",
		ToolName:  "shell",
		Request: ApprovalRequest{
			Title:       "Approve shell",
			Description: "Need approval",
			RiskLevel:   "high",
			Urgency:     "normal",
			Payload:     map[string]any{"cmd": "echo hi"},
		},
		Status: ApprovalStatusPending,
	}

	if err := logger.LogApprovalRequest(approval); err != nil {
		t.Fatalf("LogApprovalRequest: %v", err)
	}
	if err := logger.LogApprovalDecision(approval); err != nil {
		t.Fatalf("LogApprovalDecision: %v", err)
	}
	if err := logger.LogApprovalCancelled("ap-1"); err != nil {
		t.Fatalf("LogApprovalCancelled: %v", err)
	}
	if err := logger.LogApprovalExpired("ap-2"); err != nil {
		t.Fatalf("LogApprovalExpired: %v", err)
	}
	if err := logger.LogBatchApproval("batch-1", 3); err != nil {
		t.Fatalf("LogBatchApproval: %v", err)
	}
	if err := logger.LogSecurityAssessment(SecurityAssessmentResult{
		ToolName:       "shell",
		RiskLevel:      "high",
		Recommendation: "deny",
		Timestamp:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("LogSecurityAssessment: %v", err)
	}
	if err := logger.LogToolCheck("shell", ToolCheckResult{
		ToolName:  "shell",
		Approved:  false,
		Reason:    "blocked",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("LogToolCheck: %v", err)
	}
	if err := logger.LogPathCheck("/root", PathCheckResult{
		Path:      "/root",
		Protected: true,
		Reason:    "protected",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("LogPathCheck: %v", err)
	}

	input := map[string]any{"cmd": "echo hi"}
	logger.LogTool("shell", input, "ok", errors.New("boom"))
	input["cmd"] = "changed later"

	events, err := logger.Tail(20)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(events) != 9 {
		t.Fatalf("expected 9 events, got %d", len(events))
	}
	if events[0].Action != "approval_request" || events[1].Action != "approval_decision" {
		t.Fatalf("unexpected event order: %+v", events[:2])
	}
	last := events[len(events)-1]
	if last.Action != "shell" {
		t.Fatalf("expected last action shell, got %q", last.Action)
	}
	if got := last.Input["cmd"]; got != "echo hi" {
		t.Fatalf("expected cloned input, got %#v", got)
	}
	if last.Error != "boom" {
		t.Fatalf("expected error to be recorded, got %q", last.Error)
	}

	limited, err := logger.Tail(3)
	if err != nil {
		t.Fatalf("Tail limited: %v", err)
	}
	if len(limited) != 3 {
		t.Fatalf("expected 3 limited events, got %d", len(limited))
	}
}

func TestLoggerNilAndHelpers(t *testing.T) {
	var nilLogger *Logger
	if err := nilLogger.Append(Event{Action: "noop"}); err != nil {
		t.Fatalf("Append on nil logger: %v", err)
	}

	missing := New(filepath.Join(t.TempDir(), "missing.log"), "agent")
	events, err := missing.Tail(5)
	if err != nil {
		t.Fatalf("Tail on missing file: %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil events for missing file, got %+v", events)
	}

	if got := cloneMap(nil); got != nil {
		t.Fatalf("expected nil clone for nil input, got %+v", got)
	}

	lines := splitLines("a\r\nb\nc")
	if strings.Join(lines, ",") != "a,b,c" {
		t.Fatalf("unexpected split lines: %+v", lines)
	}
}
