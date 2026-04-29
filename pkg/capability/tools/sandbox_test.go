package tools

import (
	"context"
	"testing"
)

func TestSandboxManagerResolveExecutionNilManagerUsesRequestedCwd(t *testing.T) {
	var manager *SandboxManager

	cwd, factory, err := manager.ResolveExecution(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("ResolveExecution returned error: %v", err)
	}
	if cwd != "workspace" {
		t.Fatalf("expected requested cwd, got %q", cwd)
	}
	if factory != nil {
		t.Fatal("expected nil command factory without a sandbox manager")
	}
}

func TestSandboxManagerResolveExecutionNilManagerAllowsEmptyCwd(t *testing.T) {
	var manager *SandboxManager

	cwd, factory, err := manager.ResolveExecution(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveExecution returned error: %v", err)
	}
	if cwd != "" {
		t.Fatalf("expected empty cwd without a manager fallback, got %q", cwd)
	}
	if factory != nil {
		t.Fatal("expected nil command factory without a sandbox manager")
	}
}
