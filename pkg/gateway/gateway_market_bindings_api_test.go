package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestMarketBindingNormalizesMainAgentAndRefreshes(t *testing.T) {
	server := newMarketBindingTestServer(t)
	receipt := &marketplace.InstallReceipt{
		ID:            "cloud.agent.code-reviewer@1.0.0",
		ArtifactID:    "cloud.agent.code-reviewer",
		Kind:          marketplace.ArtifactKindAgent,
		Name:          "Code Reviewer",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: t.TempDir(),
		InstalledBy:   "user",
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := server.marketplaceStore().SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/bindings", strings.NewReader(`{"artifact_id":"cloud.agent.code-reviewer","target_type":"main_agent"}`))
	server.handleMarketBindings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("binding status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data marketplace.Binding `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.TargetID != "Main Agent" {
		t.Fatalf("expected main_agent target to normalize to Main Agent, got %q", payload.Data.TargetID)
	}
	if metrics := server.runtimePool.Metrics(); metrics.Refreshes != 1 {
		t.Fatalf("expected one runtime refresh, got %+v", metrics)
	}

	rec = httptest.NewRecorder()
	server.handleMarketBindings(rec, httptest.NewRequest(http.MethodGet, "/market/bindings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list bindings status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.handleMarketEvents(rec, httptest.NewRequest(http.MethodGet, "/market/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("events status = %d body=%s", rec.Code, rec.Body.String())
	}
	var events struct {
		Data marketplace.MarketEventListResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if events.Data.Total == 0 {
		t.Fatal("expected binding event")
	}

	rec = httptest.NewRecorder()
	server.handleMarketBindingByID(rec, httptest.NewRequest(http.MethodDelete, "/market/bindings/"+payload.Data.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete binding status = %d body=%s", rec.Code, rec.Body.String())
	}
	if metrics := server.runtimePool.Metrics(); metrics.Refreshes != 2 {
		t.Fatalf("expected delete to refresh runtime, got %+v", metrics)
	}
}

func TestMarketBindingWorkspaceRefreshDoesNotInvalidateOtherWorkspace(t *testing.T) {
	server := newMarketBindingTestServer(t)
	receipt := &marketplace.InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: t.TempDir(),
		InstalledBy:   "user",
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := server.marketplaceStore().SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	defaultOrg, defaultProject, defaultWorkspace := defaultResourceIDs(server.mainRuntime.WorkingDir)
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatal(err)
	}
	if err := server.store.UpsertWorkspace(&state.Workspace{ID: "workspace-2", ProjectID: defaultProject, Name: "Workspace 2", Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	server.runtimePool.Remember("Main Agent", defaultOrg, defaultProject, defaultWorkspace, &appRuntime.MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "Main Agent"}}})
	server.runtimePool.Remember("Main Agent", defaultOrg, defaultProject, "workspace-2", &appRuntime.MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "Main Agent"}}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/bindings", strings.NewReader(`{"artifact_id":"cloud.skill.release-notes","target_type":"workspace","target_id":"workspace-2"}`))
	server.handleMarketBindings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("binding status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, item := range server.runtimePool.List() {
		if item.Workspace == "workspace-2" {
			t.Fatalf("workspace-2 runtime should have been refreshed: %+v", item)
		}
	}
	foundDefault := false
	for _, item := range server.runtimePool.List() {
		if item.Workspace == defaultWorkspace {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatal("default workspace runtime should remain pooled")
	}
}

func TestMarketRefreshUsesDefaults(t *testing.T) {
	server := newMarketBindingTestServer(t)
	rec := httptest.NewRecorder()
	server.handleMarketRefresh(rec, httptest.NewRequest(http.MethodPost, "/market/refresh", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", rec.Code, rec.Body.String())
	}
	if metrics := server.runtimePool.Metrics(); metrics.Refreshes != 1 {
		t.Fatalf("expected one runtime refresh, got %+v", metrics)
	}
}

func TestMarketRefreshSupportsSessionScope(t *testing.T) {
	server := newMarketBindingTestServer(t)
	defaultOrg, defaultProject, defaultWorkspace := defaultResourceIDs(server.mainRuntime.WorkingDir)
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatal(err)
	}
	if err := server.store.UpsertWorkspace(&state.Workspace{ID: "workspace-session", ProjectID: defaultProject, Name: "Session Workspace", Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	session, err := server.sessions.Create("Session A", "Main Agent", defaultOrg, defaultProject, "workspace-session")
	if err != nil {
		t.Fatal(err)
	}
	server.runtimePool.Remember("Main Agent", defaultOrg, defaultProject, defaultWorkspace, &appRuntime.MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "Main Agent"}}})
	server.runtimePool.Remember("Main Agent", defaultOrg, defaultProject, "workspace-session", &appRuntime.MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "Main Agent"}}})

	rec := httptest.NewRecorder()
	server.handleMarketRefresh(rec, httptest.NewRequest(http.MethodPost, "/market/refresh", strings.NewReader(`{"scope":"session","session_id":"`+session.ID+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Result appRuntime.RefreshResult `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Result.Scope.Kind != appRuntime.RefreshScopeSession || payload.Result.Scope.SessionID != session.ID || payload.Result.Scope.Workspace != "workspace-session" {
		t.Fatalf("unexpected refresh result: %#v", payload.Result)
	}
	for _, item := range server.runtimePool.List() {
		if item.Workspace == "workspace-session" {
			t.Fatalf("expected session runtime refreshed, got %+v", server.runtimePool.List())
		}
	}
	foundDefault := false
	for _, item := range server.runtimePool.List() {
		if item.Workspace == defaultWorkspace {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatalf("default runtime should remain pooled, got %+v", server.runtimePool.List())
	}
}

func newMarketBindingTestServer(t *testing.T) *Server {
	t.Helper()
	workDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.Name = "Main Agent"
	cfg.Agent.ActiveProfile = "Main Agent"
	cfg.Agent.Profiles = []config.AgentProfile{{Name: "Main Agent", PermissionLevel: "limited"}}
	store, err := state.NewStore(workDir)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRef := &appRuntime.MainRuntime{
		ConfigPath: "anyclaw.json",
		Config:     cfg,
		WorkDir:    workDir,
		WorkingDir: workDir,
	}
	server := &Server{
		mainRuntime: runtimeRef,
		store:       store,
		runtimePool: appRuntime.NewRuntimePool("anyclaw.json", store, 4, time.Minute),
		marketJobs:  marketplace.NewStore(workDir),
	}
	server.sessions = state.NewSessionManager(store, nil)
	server.hotReload = appRuntime.NewHotReloadCoordinator(server.runtimePool, store)
	return server
}
