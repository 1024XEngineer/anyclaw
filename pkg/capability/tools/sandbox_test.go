package tools

import (
	"context"
	"testing"
)

func TestWithSandboxScopeSupportsNilContext(t *testing.T) {
	ctx := WithSandboxScope(nil, SandboxScope{SessionID: "s1", Channel: "chat"})
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	scope := sandboxScopeFromContext(ctx)
	if scope.SessionID != "s1" || scope.Channel != "chat" {
		t.Fatalf("unexpected scope: %#v", scope)
	}
}

func TestSandboxManagerResolveExecutionHandlesNilManager(t *testing.T) {
	var manager *SandboxManager

	cwd, launcher, err := manager.ResolveExecution(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveExecution: %v", err)
	}
	if cwd != "" {
		t.Fatalf("expected empty cwd for nil manager, got %q", cwd)
	}
	if launcher != nil {
		t.Fatal("expected nil launcher for nil manager")
	}
}

func TestSandboxManagerResolveExecutionUsesWorkingDirWhenDisabled(t *testing.T) {
	manager := &SandboxManager{workingDir: "/workspace"}

	cwd, launcher, err := manager.ResolveExecution(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveExecution: %v", err)
	}
	if cwd != "/workspace" {
		t.Fatalf("expected working dir fallback, got %q", cwd)
	}
	if launcher != nil {
		t.Fatal("expected nil launcher when sandbox is disabled")
	}
}
