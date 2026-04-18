package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/state"
)

func newAgentManagementTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workspace")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:            "Go Expert",
			Description:     "Go specialist",
			PermissionLevel: "limited",
			Enabled:         config.BoolPtr(true),
		},
	}

	configPath := filepath.Join(tempDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	store, err := state.NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	server := &Server{
		mainRuntime: &appRuntime.App{
			ConfigPath: configPath,
			Config:     cfg,
			WorkDir:    cfg.Agent.WorkDir,
			WorkingDir: cfg.Agent.WorkingDir,
		},
		store:    store,
		sessions: state.NewSessionManager(store, nil),
		bus:      state.NewEventBus(),
	}
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}

	_, _, workspaceID := defaultResourceIDs(cfg.Agent.WorkingDir)
	return server, workspaceID
}

func TestHandleAgentsAliasListsProfiles(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)
	user := &AuthUser{Name: "operator", Permissions: []string{"config.read"}}

	tests := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{name: "agents", path: "/agents", handler: server.handleAgents},
		{name: "assistants-alias", path: "/assistants", handler: server.handleAssistants},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req = req.WithContext(context.WithValue(req.Context(), authUserKey, user))
			rec := httptest.NewRecorder()

			tc.handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}

			var payload []agentProfileView
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(payload) != 1 {
				t.Fatalf("expected 1 profile, got %d", len(payload))
			}
			if payload[0].Name != "Go Expert" {
				t.Fatalf("expected Go Expert, got %q", payload[0].Name)
			}
		})
	}
}

func TestHandleSessionsAcceptsAgentField(t *testing.T) {
	server, workspaceID := newAgentManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/sessions?workspace="+workspaceID, strings.NewReader(`{"title":"Agent Session","agent":"Go Expert"}`))
	rec := httptest.NewRecorder()

	server.sessionAPI().HandleCollection(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "main agent") {
		t.Fatalf("expected main-agent-only error, got %s", rec.Body.String())
	}
}

func TestHandleSessionsAcceptsMainAliasField(t *testing.T) {
	server, workspaceID := newAgentManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/sessions?workspace="+workspaceID, strings.NewReader(`{"title":"Main Session","agent":"main"}`))
	rec := httptest.NewRecorder()

	server.sessionAPI().HandleCollection(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var session state.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if session.Agent != server.mainRuntime.Config.ResolveMainAgentName() {
		t.Fatalf("expected session agent %q, got %q", server.mainRuntime.Config.ResolveMainAgentName(), session.Agent)
	}
	if session.Title != "Main Session" {
		t.Fatalf("expected session title Main Session, got %q", session.Title)
	}
}

func TestHandleChatRejectsSpecialistField(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","agent":"Go Expert"}`))
	rec := httptest.NewRecorder()

	server.handleChat(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "main agent") {
		t.Fatalf("expected main-agent-only error, got %s", rec.Body.String())
	}
}

func TestHandleV2TaskCreateRejectsSpecialistSelection(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v2/tasks", strings.NewReader(`{"input":"ship it","selected_agent":"Go Expert"}`))
	rec := httptest.NewRecorder()

	server.handleV2TaskCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "main agent") {
		t.Fatalf("expected main-agent-only error, got %s", rec.Body.String())
	}
}

func TestV2VisibleAgentsMarksSpecialistsAsInternalOnly(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v2/agents", nil)
	rec := httptest.NewRecorder()

	server.handleV2Agents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	records := map[string]map[string]any{}
	for _, item := range payload {
		name, _ := item["name"].(string)
		records[name] = item
	}

	mainName := server.mainRuntime.Config.ResolveMainAgentName()
	mainRecord, ok := records[mainName]
	if !ok {
		t.Fatalf("expected main agent %q in payload %#v", mainName, payload)
	}
	if publicEntry, _ := mainRecord["public_entry"].(bool); !publicEntry {
		t.Fatalf("expected main agent to be public entry, got %#v", mainRecord)
	}
	if entry, _ := mainRecord["entry"].(string); entry != "main" {
		t.Fatalf("expected main entry marker, got %#v", mainRecord)
	}

	specialistRecord, ok := records["Go Expert"]
	if !ok {
		t.Fatalf("expected specialist in payload %#v", payload)
	}
	if publicEntry, _ := specialistRecord["public_entry"].(bool); publicEntry {
		t.Fatalf("expected specialist to be internal-only, got %#v", specialistRecord)
	}
	if entry, _ := specialistRecord["entry"].(string); entry != "specialist" {
		t.Fatalf("expected specialist entry marker, got %#v", specialistRecord)
	}
}
