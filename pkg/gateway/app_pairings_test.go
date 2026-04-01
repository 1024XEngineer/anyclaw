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

func TestHandleAppPairingsCreateAndList(t *testing.T) {
	baseDir := t.TempDir()
	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	server := &Server{
		store: store,
		app:   &appRuntime.App{ConfigPath: filepath.Join(baseDir, "anyclaw.json")},
	}

	postReq := httptest.NewRequest(http.MethodPost, "/app-pairings", strings.NewReader(`{
	  "app":"qq-local",
	  "workflow":"send-message",
	  "name":"personal-chat",
	  "binding":"primary",
	  "triggers":["给张三发消息"],
	  "defaults":{"contact":"张三","human_like":true}
	}`))
	postReq = postReq.WithContext(context.WithValue(postReq.Context(), authUserKey, &AuthUser{
		Name:        "operator",
		Permissions: []string{"apps.write"},
	}))
	postRec := httptest.NewRecorder()

	server.handleAppPairings(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/app-pairings?app=qq-local", nil)
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"apps.read"},
	}))
	getRec := httptest.NewRecorder()

	server.handleAppPairings(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var payload []appPairingView
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 pairing, got %d", len(payload))
	}
	if payload[0].Workflow != "send-message" {
		t.Fatalf("expected workflow send-message, got %q", payload[0].Workflow)
	}
	if payload[0].Binding != "primary" {
		t.Fatalf("expected binding primary, got %q", payload[0].Binding)
	}
	if payload[0].Defaults["contact"] != "张三" {
		t.Fatalf("expected defaults to round-trip, got %#v", payload[0].Defaults)
	}
}
