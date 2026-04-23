package state

import (
	"strings"
	"testing"
	"time"
)

func TestApprovalManagerRequestWithSignaturePreservesPayloadAndClonesInput(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	manager := NewApprovalManager(store)
	manager.nextID = func() string { return "approval-1" }
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	manager.nowFunc = func() time.Time { return now }

	payload := map[string]any{"message": "original"}
	signaturePayload := map[string]any{"message": "signed"}

	approval, err := manager.RequestWithSignature(" task-1 ", " session-1 ", 3, " shell ", " execute ", payload, signaturePayload)
	if err != nil {
		t.Fatalf("RequestWithSignature: %v", err)
	}

	payload["message"] = "mutated"
	signaturePayload["message"] = "changed-after-sign"

	if approval.ID != "approval-1" {
		t.Fatalf("expected deterministic approval ID, got %q", approval.ID)
	}
	if approval.TaskID != "task-1" || approval.SessionID != "session-1" {
		t.Fatalf("expected trimmed task/session IDs, got %#v", approval)
	}
	if approval.ToolName != "shell" || approval.Action != "execute" {
		t.Fatalf("expected trimmed tool/action, got %#v", approval)
	}
	if approval.Payload["message"] != "original" {
		t.Fatalf("expected cloned payload to keep original content, got %#v", approval.Payload)
	}
	if approval.Signature != `shell|execute|{"message":"signed"}` {
		t.Fatalf("expected signature to use signature payload, got %q", approval.Signature)
	}
	if !approval.RequestedAt.Equal(now) {
		t.Fatalf("expected requested time %v, got %v", now, approval.RequestedAt)
	}

	stored, ok := store.GetApproval("approval-1")
	if !ok || stored == nil {
		t.Fatal("expected approval to be stored")
	}
	if stored.Payload["message"] != "original" {
		t.Fatalf("expected stored payload to be isolated from input mutations, got %#v", stored.Payload)
	}
	if stored.Signature != approval.Signature {
		t.Fatalf("expected stored signature %q, got %q", approval.Signature, stored.Signature)
	}
}

func TestApprovalManagerResolveUpdatesPendingApprovalAndIsIdempotent(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Date(2026, 4, 22, 12, 30, 0, 0, time.UTC)
	approval := &Approval{
		ID:          "approval-2",
		SessionID:   "sess-1",
		ToolName:    "shell",
		Action:      "execute",
		Signature:   "sig",
		Status:      "pending",
		RequestedAt: now.Add(-time.Minute),
	}
	if err := store.AppendApproval(approval); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}

	manager := NewApprovalManager(store)
	manager.nowFunc = func() time.Time { return now }

	resolved, err := manager.Resolve("approval-2", true, " reviewer ", " approved ")
	if err != nil {
		t.Fatalf("Resolve(first): %v", err)
	}
	if resolved.Status != "approved" {
		t.Fatalf("expected approved status, got %q", resolved.Status)
	}
	if resolved.ResolvedBy != "reviewer" || resolved.Comment != "approved" {
		t.Fatalf("expected trimmed resolution metadata, got %#v", resolved)
	}
	if resolved.ResolvedAt != now.Format(time.RFC3339) {
		t.Fatalf("expected resolved time %q, got %q", now.Format(time.RFC3339), resolved.ResolvedAt)
	}

	second, err := manager.Resolve("approval-2", false, "other", "overwrite")
	if err != nil {
		t.Fatalf("Resolve(second): %v", err)
	}
	if second.Status != "approved" {
		t.Fatalf("expected idempotent status to stay approved, got %q", second.Status)
	}
	if second.ResolvedBy != "reviewer" || second.Comment != "approved" || second.ResolvedAt != resolved.ResolvedAt {
		t.Fatalf("expected second resolve to preserve original resolution, got %#v", second)
	}
}

func TestApprovalManagerResolveRejectsMissingApproval(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	manager := NewApprovalManager(store)
	_, err = manager.Resolve("missing", false, "actor", "comment")
	if err == nil {
		t.Fatal("expected missing approval to return an error")
	}
	if !strings.Contains(err.Error(), "approval not found: missing") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
