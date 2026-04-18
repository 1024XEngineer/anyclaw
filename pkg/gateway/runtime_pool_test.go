package gateway

import (
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestRuntimePoolGetOrCreateUsesFullHierarchyKey(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&state.Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&state.Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if err := store.UpsertWorkspace(&state.Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: t.TempDir()}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	mainRuntime := &appRuntime.App{
		Config:     &config.Config{Agent: config.AgentConfig{Name: "assistant"}},
		WorkingDir: "D:/workspace",
		WorkDir:    "D:/workdir",
	}
	pool := appRuntime.NewRuntimePool("ignored", store, 4, time.Hour)
	pool.Remember("assistant", "org-1", "project-1", "workspace-1", mainRuntime)

	got, err := pool.GetOrCreate("assistant", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if got != mainRuntime {
		t.Fatal("expected cached runtime entry to be reused")
	}
}

func TestRuntimePoolListShowsWorkspaceIDAndPath(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	pool := appRuntime.NewRuntimePool("ignored", store, 4, time.Hour)
	pool.Remember("assistant", "org-1", "project-1", "workspace-1", &appRuntime.App{
		Config:     &config.Config{Agent: config.AgentConfig{Name: "assistant"}},
		WorkingDir: "D:/workspace",
		WorkDir:    "D:/workdir",
	})

	items := pool.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 runtime, got %d", len(items))
	}
	if items[0].Workspace != "workspace-1" {
		t.Fatalf("expected workspace id workspace-1, got %q", items[0].Workspace)
	}
	if items[0].WorkspacePath != "D:/workspace" {
		t.Fatalf("expected workspace path D:/workspace, got %q", items[0].WorkspacePath)
	}
}
