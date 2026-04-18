package transport

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestSummarizeApprovalsIgnoresOrphanedSessionApprovals(t *testing.T) {
	summary := summarizeApprovals([]*state.Approval{
		{ID: "approval-live", SessionID: "sess-live", Status: "pending"},
		{ID: "approval-orphan", SessionID: "sess-missing", Status: "pending"},
		{ID: "approval-rejected", SessionID: "sess-live", Status: "rejected"},
	}, []*state.Session{
		{ID: "sess-live"},
	}, nil)

	if summary.Pending != 1 {
		t.Fatalf("expected 1 pending actionable approval, got %#v", summary)
	}
	if summary.Denied != 1 {
		t.Fatalf("expected rejected approval to count as denied, got %#v", summary)
	}
	if summary.Total != 2 {
		t.Fatalf("expected orphaned approval to be excluded from totals, got %#v", summary)
	}
}
