package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestHotReloadCoordinatorRefreshesScopedRuntime(t *testing.T) {
	store, _ := newRuntimePoolTestStore(t)
	pool := NewRuntimePool("anyclaw.json", store, 4, time.Hour)
	pool.Remember("agent-1", "org-1", "project-1", "workspace-1", &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-1"}}})
	pool.Remember("agent-2", "org-1", "project-2", "workspace-2", &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-2"}}})

	result := NewHotReloadCoordinator(pool, store).Refresh(context.Background(), RefreshScope{
		Kind:      RefreshScopeRuntime,
		Agent:     "agent-1",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if result.Status != "refreshed" {
		t.Fatalf("result = %#v, want refreshed", result)
	}
	if _, ok := pool.runtimes[runtimeKey("agent-1", "org-1", "project-1", "workspace-1")]; ok {
		t.Fatal("expected scoped runtime to be refreshed")
	}
	if _, ok := pool.runtimes[runtimeKey("agent-2", "org-1", "project-2", "workspace-2")]; !ok {
		t.Fatal("other runtime should remain pooled")
	}
	if metrics := pool.Metrics(); metrics.Refreshes != 1 {
		t.Fatalf("metrics = %+v, want one refresh", metrics)
	}
}

func TestHotReloadCoordinatorSessionScopeUsesExecutionBinding(t *testing.T) {
	store, sessions := newRuntimePoolTestStore(t)
	pool := NewRuntimePool("anyclaw.json", store, 4, time.Hour)
	session, err := sessions.CreateWithOptions(state.SessionCreateOptions{
		Title:     "session",
		AgentName: "agent-1",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	pool.Remember("agent-1", "org-1", "project-1", "workspace-1", &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-1"}}})

	result := NewHotReloadCoordinator(pool, store).Refresh(context.Background(), RefreshScope{
		Kind:      RefreshScopeSession,
		SessionID: session.ID,
	})
	if result.Status != "refreshed" || result.Scope.Workspace != "workspace-1" || result.Scope.Agent != "agent-1" {
		t.Fatalf("result = %#v, want session binding", result)
	}
	if _, ok := pool.runtimes[runtimeKey("agent-1", "org-1", "project-1", "workspace-1")]; ok {
		t.Fatal("expected session runtime to be refreshed")
	}
}

func TestHotReloadCoordinatorIsolatesFailures(t *testing.T) {
	store, _ := newRuntimePoolTestStore(t)
	pool := NewRuntimePool("anyclaw.json", store, 4, time.Hour)
	pool.Remember("agent-1", "org-1", "project-1", "workspace-1", &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-1"}}})

	results := NewHotReloadCoordinator(pool, store).RefreshMany(context.Background(), []RefreshScope{
		{Kind: RefreshScopeRuntime, Agent: "agent-1", Org: "org-1", Project: "project-1", Workspace: "workspace-1"},
		{Kind: RefreshScopeRuntime, Agent: "agent-2", Org: "org-1", Project: "project-2"},
	})
	if len(results) != 2 || results[0].Status != "refreshed" || results[1].Status != "failed" {
		t.Fatalf("results = %#v, want isolated success/failure", results)
	}
	if metrics := pool.Metrics(); metrics.Refreshes != 1 {
		t.Fatalf("metrics = %+v, want only successful scope counted", metrics)
	}
}
