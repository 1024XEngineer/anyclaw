package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/gorilla/websocket"
)

func TestOpenClawWSResolvesAppWorkflows(t *testing.T) {
	baseDir := t.TempDir()
	cfg := config.DefaultConfig()

	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	server := &Server{
		app: &appRuntime.App{
			Config:  cfg,
			Plugins: newWorkflowRegistryForTest(t),
			WorkDir: baseDir,
		},
		store:    store,
		sessions: NewSessionManager(store, nil),
		bus:      NewBus(),
		auth:     newAuthMiddleware(&cfg.Security),
		plugins:  newWorkflowRegistryForTest(t),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.wrap("/ws", server.handleOpenClawWS))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var challenge openClawWSFrame
	if err := conn.ReadJSON(&challenge); err != nil {
		t.Fatalf("ReadJSON challenge: %v", err)
	}
	challengeData, ok := challenge.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected challenge data map, got %#v", challenge.Data)
	}
	nonce, _ := challengeData["nonce"].(string)
	if nonce == "" {
		t.Fatalf("expected nonce in challenge frame: %#v", challengeData)
	}

	if err := conn.WriteJSON(openClawWSFrame{
		Type:   "req",
		ID:     "connect-1",
		Method: "connect",
		Params: map[string]any{"challenge": nonce},
	}); err != nil {
		t.Fatalf("WriteJSON connect: %v", err)
	}

	var connected openClawWSFrame
	if err := conn.ReadJSON(&connected); err != nil {
		t.Fatalf("ReadJSON connected: %v", err)
	}
	if connected.Type != "res" || !connected.OK {
		t.Fatalf("expected successful connect response, got %#v", connected)
	}

	if err := conn.WriteJSON(openClawWSFrame{
		Type:   "req",
		ID:     "workflow-resolve-1",
		Method: "app-workflows.resolve",
		Params: map[string]any{"q": "remove background from image", "limit": 2},
	}); err != nil {
		t.Fatalf("WriteJSON app-workflows.resolve: %v", err)
	}

	var resolved openClawWSFrame
	if err := conn.ReadJSON(&resolved); err != nil {
		t.Fatalf("ReadJSON app-workflows.resolve: %v", err)
	}
	if resolved.Type != "res" || !resolved.OK {
		t.Fatalf("expected successful workflow resolve response, got %#v", resolved)
	}
	payload, ok := resolved.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected workflow resolve payload map, got %#v", resolved.Data)
	}
	matches, ok := payload["matches"].([]any)
	if !ok || len(matches) == 0 {
		t.Fatalf("expected workflow matches, got %#v", payload)
	}
}
