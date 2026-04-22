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

func TestRequestToolApprovalWithoutHook(t *testing.T) {
	if err := RequestToolApproval(nil, "noop", nil); err != nil {
		t.Fatalf("expected nil context approval to be ignored, got %v", err)
	}
	if err := RequestToolApproval(context.Background(), "noop", nil); err != nil {
		t.Fatalf("expected missing approval hook to be ignored, got %v", err)
	}
}

func TestToolCallerContextHelpers(t *testing.T) {
	caller := ToolCaller{
		Role:        ToolCallerRoleSubAgent,
		AgentName:   "helper",
		ExecutionID: "exec-1",
	}
	ctx := WithToolCaller(nil, caller)
	if got := ToolCallerFromContext(ctx); got != caller {
		t.Fatalf("unexpected caller from context: %#v", got)
	}
	if got := ToolCallerFromContext(nil); got != (ToolCaller{}) {
		t.Fatalf("expected zero caller from nil context, got %#v", got)
	}
	if WithToolApprovalHook(context.Background(), nil) == nil {
		t.Fatal("expected WithToolApprovalHook to preserve context when hook is nil")
	}
}
