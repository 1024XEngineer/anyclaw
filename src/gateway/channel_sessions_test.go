package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestSessionManagerCreateWithOptionsStripsGroupMetadata(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:        "legacy group",
		AgentName:    "AgentOne",
		Participants: []string{"AgentOne", "AgentTwo"},
		Org:          "org-1",
		Project:      "project-1",
		Workspace:    "workspace-1",
		SessionMode:  "channel-group",
		GroupKey:     "group-key",
		IsGroup:      true,
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}
	if session.Agent != "AgentOne" {
		t.Fatalf("expected primary agent AgentOne, got %q", session.Agent)
	}
	if len(session.Participants) != 0 {
		t.Fatalf("expected participants to be stripped, got %+v", session.Participants)
	}
	if session.IsGroup {
		t.Fatal("expected group mode to be stripped")
	}
	if session.GroupKey != "" {
		t.Fatalf("expected group key to be stripped, got %q", session.GroupKey)
	}
	if session.SessionMode != "main" {
		t.Fatalf("expected session mode to normalize to main, got %q", session.SessionMode)
	}
}

func TestHandleSessionsRejectsGroupCreation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if err := store.UpsertWorkspace(&Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: t.TempDir()}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	server := &Server{
		app: &appRuntime.App{
			Config: &config.Config{
				Agent: config.AgentConfig{
					Name: "AgentOne",
					Profiles: []config.AgentProfile{
						{Name: "AgentOne", Enabled: config.BoolPtr(true)},
						{Name: "AgentTwo", Enabled: config.BoolPtr(true)},
					},
				},
			},
		},
		store:    store,
		sessions: NewSessionManager(store, nil),
	}

	body, err := json.Marshal(map[string]any{
		"title":        "group",
		"assistant":    "AgentOne",
		"participants": []string{"AgentOne", "AgentTwo"},
		"is_group":     true,
		"session_mode": "channel-group",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/sessions?workspace=workspace-1", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleSessions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d with body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !bytes.Contains([]byte(got), []byte("multi-agent")) {
		t.Fatalf("expected multi-agent rejection message, got %s", got)
	}
}
