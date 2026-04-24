package runtime

import (
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestRuntimePoolGetOrCreateUsesCachedRuntimeAndTracksSessionCounts(t *testing.T) {
	store, sessions := newRuntimePoolTestStore(t)
	pool := NewRuntimePool("anyclaw.json", store, 4, time.Hour)

	runtime := &MainRuntime{
		Config:     &config.Config{Agent: config.AgentConfig{Name: "agent-1"}},
		WorkingDir: "/workspace/runtime-1",
		WorkDir:    "/work/runtime-1",
	}
	pool.Remember("agent-1", "org-1", "project-1", "workspace-1", runtime)

	if _, err := sessions.Create("Session A", "agent-1", "org-1", "project-1", "workspace-1"); err != nil {
		t.Fatalf("Create session A: %v", err)
	}
	if _, err := sessions.Create("Session B", "agent-1", "org-1", "project-1", "workspace-1"); err != nil {
		t.Fatalf("Create session B: %v", err)
	}

	got, err := pool.GetOrCreate("agent-1", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if got != runtime {
		t.Fatal("expected cached runtime to be returned")
	}

	items := pool.List()
	if len(items) != 1 {
		t.Fatalf("expected one pooled runtime, got %d", len(items))
	}
	if items[0].SessionCount != 2 {
		t.Fatalf("expected session count 2, got %d", items[0].SessionCount)
	}
	if items[0].LastReason != "cache-hit" || items[0].Hits != 1 {
		t.Fatalf("expected cache-hit metadata, got %+v", items[0])
	}

	metrics := pool.Metrics()
	if metrics.Hits != 1 {
		t.Fatalf("expected one cache hit metric, got %+v", metrics)
	}
	status := pool.Status()
	if status.Pooled != 1 || status.Idle != 1 || status.Max != 4 {
		t.Fatalf("unexpected pool status: %+v", status)
	}
}

func TestRuntimePoolInvalidationRefreshAndCleanup(t *testing.T) {
	store, _ := newRuntimePoolTestStore(t)
	pool := NewRuntimePool("anyclaw.json", store, 2, time.Minute)

	first := &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-1"}}, WorkingDir: "/workspace/1", WorkDir: "/work/1"}
	second := &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-2"}}, WorkingDir: "/workspace/2", WorkDir: "/work/2"}
	third := &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-3"}}, WorkingDir: "/workspace/3", WorkDir: "/work/3"}

	pool.Remember("agent-1", "org-1", "project-1", "workspace-1", first)
	pool.Remember("agent-2", "org-1", "project-2", "workspace-2", second)
	pool.Remember("agent-3", "org-2", "project-3", "workspace-3", third)

	pool.Refresh("agent-1", "org-1", "project-1", "workspace-1")
	if _, ok := pool.runtimes[runtimeKey("agent-1", "org-1", "project-1", "workspace-1")]; ok {
		t.Fatal("expected refresh to invalidate cached runtime")
	}
	if metrics := pool.Metrics(); metrics.Refreshes != 1 {
		t.Fatalf("expected one refresh metric, got %+v", metrics)
	}

	pool.InvalidateByAgent("agent-2")
	if _, ok := pool.runtimes[runtimeKey("agent-2", "org-1", "project-2", "workspace-2")]; ok {
		t.Fatal("expected invalidate by agent to remove runtime")
	}

	pool.Remember("agent-2", "org-1", "project-2", "workspace-2", second)
	pool.InvalidateByWorkspace("workspace-3")
	if _, ok := pool.runtimes[runtimeKey("agent-3", "org-2", "project-3", "workspace-3")]; ok {
		t.Fatal("expected invalidate by workspace to remove runtime")
	}

	pool.Remember("agent-3", "org-2", "project-3", "workspace-3", third)
	pool.InvalidateByProject("project-2")
	if _, ok := pool.runtimes[runtimeKey("agent-2", "org-1", "project-2", "workspace-2")]; ok {
		t.Fatal("expected invalidate by project to remove runtime")
	}

	now := time.Now().UTC()
	pool.runtimes[runtimeKey("agent-3", "org-2", "project-3", "workspace-3")].lastUsedAt = now.Add(-2 * time.Minute)
	pool.cleanupLocked()
	if _, ok := pool.runtimes[runtimeKey("agent-3", "org-2", "project-3", "workspace-3")]; ok {
		t.Fatal("expected cleanup to evict expired runtime")
	}
	if metrics := pool.Metrics(); metrics.Evictions == 0 {
		t.Fatalf("expected cleanup eviction metric, got %+v", metrics)
	}

	pool.Remember("agent-1", "org-1", "project-1", "workspace-1", first)
	pool.Remember("agent-2", "org-1", "project-2", "workspace-2", second)
	pool.InvalidateAll()
	if len(pool.runtimes) != 0 {
		t.Fatalf("expected all runtimes to be invalidated, got %d", len(pool.runtimes))
	}
}

func TestRuntimePoolEvictsOldestAndReportsMissingWorkspace(t *testing.T) {
	store, _ := newRuntimePoolTestStore(t)
	pool := NewRuntimePool("anyclaw.json", store, 2, time.Hour)

	firstKey := runtimeKey("agent-1", "org-1", "project-1", "workspace-1")
	secondKey := runtimeKey("agent-2", "org-1", "project-2", "workspace-2")
	pool.runtimes[firstKey] = &runtimeEntry{
		runtime:    &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-1"}}},
		createdAt:  time.Now().UTC().Add(-2 * time.Hour),
		lastUsedAt: time.Now().UTC().Add(-2 * time.Hour),
	}
	pool.runtimes[secondKey] = &runtimeEntry{
		runtime:    &MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "agent-2"}}},
		createdAt:  time.Now().UTC().Add(-time.Hour),
		lastUsedAt: time.Now().UTC().Add(-time.Hour),
	}

	pool.evictOldestLocked()
	if _, ok := pool.runtimes[firstKey]; ok {
		t.Fatal("expected oldest runtime to be evicted")
	}
	if metrics := pool.Metrics(); metrics.Evictions == 0 {
		t.Fatalf("expected eviction metric to increment, got %+v", metrics)
	}

	if _, err := pool.GetOrCreate("agent-x", "org-x", "project-x", "missing-workspace"); err == nil {
		t.Fatal("expected missing workspace error")
	}
}

func TestRuntimePoolHelpers(t *testing.T) {
	session := &state.Session{
		Agent:     "fallback-agent",
		Org:       "fallback-org",
		Project:   "fallback-project",
		Workspace: "fallback-workspace",
	}
	binding := sessionExecutionBindingValue(session)
	if binding.Agent != "fallback-agent" || binding.Org != "fallback-org" || binding.Project != "fallback-project" || binding.Workspace != "fallback-workspace" {
		t.Fatalf("expected fallback session execution binding, got %+v", binding)
	}

	key := runtimeKey("agent-1", "org-1", "project-1", "workspace-1")
	if runtimePart(key, 0) != "agent-1" || runtimePart(key, 3) != "workspace-1" || runtimePart(key, 9) != "" {
		t.Fatalf("unexpected runtime key parsing for %q", key)
	}
}

func newRuntimePoolTestStore(t *testing.T) (*state.Store, *state.SessionManager) {
	t.Helper()

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	must := func(err error) {
		if err != nil {
			t.Fatalf("seed store: %v", err)
		}
	}

	must(store.UpsertOrg(&state.Org{ID: "org-1", Name: "Org 1"}))
	must(store.UpsertOrg(&state.Org{ID: "org-2", Name: "Org 2"}))
	must(store.UpsertProject(&state.Project{ID: "project-1", OrgID: "org-1", Name: "Project 1"}))
	must(store.UpsertProject(&state.Project{ID: "project-2", OrgID: "org-1", Name: "Project 2"}))
	must(store.UpsertProject(&state.Project{ID: "project-3", OrgID: "org-2", Name: "Project 3"}))
	must(store.UpsertWorkspace(&state.Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace 1", Path: t.TempDir()}))
	must(store.UpsertWorkspace(&state.Workspace{ID: "workspace-2", ProjectID: "project-2", Name: "Workspace 2", Path: t.TempDir()}))
	must(store.UpsertWorkspace(&state.Workspace{ID: "workspace-3", ProjectID: "project-3", Name: "Workspace 3", Path: t.TempDir()}))

	return store, state.NewSessionManager(store, nil)
}
