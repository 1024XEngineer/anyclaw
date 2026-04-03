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

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	server := &Server{
		app: &appRuntime.App{
			ConfigPath: configPath,
			Config:     cfg,
			WorkDir:    cfg.Agent.WorkDir,
			WorkingDir: cfg.Agent.WorkingDir,
		},
		store:    store,
		sessions: NewSessionManager(store, nil),
		bus:      NewBus(),
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

	server.handleSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var session Session
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if session.Agent != "Go Expert" {
		t.Fatalf("expected session agent Go Expert, got %q", session.Agent)
	}
	if session.Title != "Agent Session" {
		t.Fatalf("expected session title Agent Session, got %q", session.Title)
	}
}
