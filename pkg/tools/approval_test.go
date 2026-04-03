package tools

import (
	"context"
	"testing"
)

func TestRequestToolApprovalInvokesHook(t *testing.T) {
	var called ToolApprovalCall
	ctx := WithToolApprovalHook(context.Background(), func(ctx context.Context, call ToolApprovalCall) error {
		called = call
		return nil
	})

	if err := RequestToolApproval(ctx, "desktop_plan", map[string]any{"summary": "demo"}); err != nil {
		t.Fatalf("RequestToolApproval: %v", err)
	}
	if called.Name != "desktop_plan" {
		t.Fatalf("expected desktop_plan, got %q", called.Name)
	}
	if called.Args["summary"] != "demo" {
		t.Fatalf("unexpected args: %#v", called.Args)
	}
}
