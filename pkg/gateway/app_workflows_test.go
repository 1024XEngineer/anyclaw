package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appstore "github.com/anyclaw/anyclaw/pkg/apps"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestHandleAppWorkflowResolveReturnsMatches(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := &Server{
		plugins: newWorkflowRegistryForTest(t),
		store:   store,
	}
	req := httptest.NewRequest(http.MethodGet, "/app-workflows/resolve?q=remove+background+from+image&limit=2", nil)
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"apps.read"},
	}))
	rec := httptest.NewRecorder()

	server.handleAppWorkflowResolve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload appWorkflowResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Query != "remove background from image" {
		t.Fatalf("unexpected query: %q", payload.Query)
	}
	if len(payload.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(payload.Matches))
	}
	if payload.Matches[0].ToolName != "app_image_app_workflow_remove_background" {
		t.Fatalf("unexpected tool name: %q", payload.Matches[0].ToolName)
	}
	if payload.Matches[0].Score <= 0 {
		t.Fatalf("expected positive score, got %d", payload.Matches[0].Score)
	}
}

func TestHandleAppWorkflowResolveRequiresQuery(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/app-workflows/resolve", nil)
	rec := httptest.NewRecorder()

	server.handleAppWorkflowResolve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAppWorkflowResolveReturnsPairingHints(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	appStore, err := newAppStore(configPath)
	if err != nil {
		t.Fatalf("newAppStore: %v", err)
	}
	if err := appStore.UpsertPairing(&appstore.Pairing{
		ID:       "pair-1",
		App:      "image-app",
		Workflow: "remove-background",
		Name:     "local-cutout",
		Binding:  "primary",
		Triggers: []string{"cutout", "remove background"},
		Defaults: map[string]any{"export_format": "png"},
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertPairing: %v", err)
	}
	server := &Server{
		plugins: newWorkflowRegistryForTest(t),
		store:   store,
		app:     &appRuntime.App{ConfigPath: configPath},
	}
	req := httptest.NewRequest(http.MethodGet, "/app-workflows/resolve?q=cutout+this+image&limit=2", nil)
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"apps.read"},
	}))
	rec := httptest.NewRecorder()

	server.handleAppWorkflowResolve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload appWorkflowResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Matches) == 0 {
		t.Fatal("expected pairing-aware matches")
	}
	if payload.Matches[0].Pairing == nil || payload.Matches[0].Pairing.Name != "local-cutout" {
		t.Fatalf("expected pairing hint in response, got %#v", payload.Matches[0])
	}
	if payload.Matches[0].Pairing.Binding != "primary" {
		t.Fatalf("expected pairing binding in response, got %#v", payload.Matches[0].Pairing)
	}
}
