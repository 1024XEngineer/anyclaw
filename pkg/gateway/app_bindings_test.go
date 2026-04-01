package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestHandleAppBindingsCreateAndList(t *testing.T) {
	baseDir := t.TempDir()
	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	server := &Server{
		store: store,
		app:   &appRuntime.App{ConfigPath: filepath.Join(baseDir, "anyclaw.json")},
	}

	postReq := httptest.NewRequest(http.MethodPost, "/app-bindings", strings.NewReader(`{
	  "app":"demo-app",
	  "name":"primary",
	  "config":{"base_url":"https://example.com"},
	  "secrets":{"token":"secret-value"}
	}`))
	postReq = postReq.WithContext(context.WithValue(postReq.Context(), authUserKey, &AuthUser{
		Name:        "operator",
		Permissions: []string{"apps.write"},
	}))
	postRec := httptest.NewRecorder()

	server.handleAppBindings(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/app-bindings?app=demo-app", nil)
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"apps.read"},
	}))
	getRec := httptest.NewRecorder()

	server.handleAppBindings(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload []appBindingView
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(payload))
	}
	if payload[0].App != "demo-app" {
		t.Fatalf("expected app demo-app, got %q", payload[0].App)
	}
	if !payload[0].HasSecrets {
		t.Fatal("expected binding to report stored secrets")
	}
	if payload[0].Config["base_url"] != "https://example.com" {
		t.Fatalf("expected config base_url to round-trip")
	}
}
