package gateway

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func newTestMainRuntime(t *testing.T) *runtime.MainRuntime {
	t.Helper()

	workDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 0
	cfg.Agent.Name = "main"

	return &runtime.MainRuntime{
		ConfigPath: filepath.Join(workDir, "anyclaw.json"),
		Config:     cfg,
		WorkDir:    workDir,
		WorkingDir: workDir,
	}
}

func TestNewGatewayDefaults(t *testing.T) {
	runtimeRef := newTestMainRuntime(t)
	server := New(runtimeRef)
	if server == nil {
		t.Fatal("expected server")
	}
	if server.mainRuntime != runtimeRef {
		t.Fatalf("expected runtime reference to be retained")
	}
	if got := server.address(); got != runtime.GatewayAddress(runtimeRef.Config) {
		t.Fatalf("expected runtime address %q, got %q", runtime.GatewayAddress(runtimeRef.Config), got)
	}
}

func TestGatewayAddressValidation(t *testing.T) {
	if got := (*Server)(nil).address(); got != defaultGatewayAddress {
		t.Fatalf("expected nil server to use default address, got %q", got)
	}
	if got := (&Server{}).address(); got != defaultGatewayAddress {
		t.Fatalf("expected empty server to use default address, got %q", got)
	}
}

func TestGatewayRunNilServer(t *testing.T) {
	var server *Server
	if err := server.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil server error, got %v", err)
	}
}

func TestGatewayRunReportsListenFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	runtimeRef := newTestMainRuntime(t)
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	runtimeRef.Config.Gateway.Host = host
	runtimeRef.Config.Gateway.Port = parseIntParam(port, 0)
	server := New(runtimeRef)
	err = server.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gateway server failed") {
		t.Fatalf("expected listen failure, got %v", err)
	}
}

func TestGatewayRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := New(newTestMainRuntime(t))
	if err := server.Run(ctx); err != nil {
		t.Fatalf("run with canceled context: %v", err)
	}
	if server.httpServer == nil {
		t.Fatal("expected http server to be initialized")
	}
	if server.startedAt.IsZero() {
		t.Fatal("expected startedAt to be set")
	}
}

func TestGatewayRunStartsWorkers(t *testing.T) {
	server := New(newTestMainRuntime(t))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("run stopped with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("gateway run did not stop")
		}
	}()

	ran := make(chan struct{}, 1)
	server.jobQueue <- func() {
		select {
		case ran <- struct{}{}:
		default:
		}
	}

	select {
	case <-ran:
	case err := <-errCh:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("gateway worker did not execute queued jobs")
	}
}

func TestRegisterGatewayRoutesMountsGatewayAPIs(t *testing.T) {
	server := New(newTestMainRuntime(t))
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}

	mux := http.NewServeMux()
	server.registerGatewayRoutes(mux)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "health", method: http.MethodGet, path: "/healthz", want: http.StatusOK},
		{name: "status", method: http.MethodGet, path: "/status", want: http.StatusOK},
		{name: "events", method: http.MethodGet, path: "/events", want: http.StatusOK},
		{name: "approvals", method: http.MethodGet, path: "/approvals", want: http.StatusOK},
		{name: "resources", method: http.MethodGet, path: "/resources", want: http.StatusOK},
		{name: "runtimes", method: http.MethodGet, path: "/runtimes", want: http.StatusOK},
		{name: "control plane", method: http.MethodGet, path: "/control-plane", want: http.StatusOK},
		{name: "tasks", method: http.MethodGet, path: "/tasks", want: http.StatusOK},
		{name: "sessions", method: http.MethodGet, path: "/sessions", want: http.StatusOK},
		{name: "nodes", method: http.MethodGet, path: "/nodes", want: http.StatusOK},
		{name: "discovery", method: http.MethodGet, path: "/discovery/instances", want: http.StatusOK},
		{name: "models", method: http.MethodGet, path: "/v1/models", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("%s %s = %d, want %d; body=%s", tt.method, tt.path, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestGatewayRootAdvertisesMountedEndpoints(t *testing.T) {
	server := New(newTestMainRuntime(t))
	mux := http.NewServeMux()
	server.registerGatewayRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode root payload: %v", err)
	}

	for key, want := range map[string]string{
		"health":        "/healthz",
		"status":        "/status",
		"events":        "/events",
		"approvals":     "/approvals",
		"resources":     "/resources",
		"runtimes":      "/runtimes",
		"control_plane": "/control-plane",
		"tasks":         "/tasks",
		"sessions":      "/sessions",
		"nodes":         "/nodes",
		"discovery":     "/discovery/instances",
		"openai_api":    "/v1/chat/completions",
		"models":        "/v1/models",
		"responses":     "/v1/responses",
	} {
		if got := payload.Endpoints[key]; got != want {
			t.Fatalf("root endpoint %q = %q, want %q", key, got, want)
		}
	}

	for _, key := range []string{"chat", "agents", "channels", "plugins", "skills", "tools", "websocket", "webhooks", "cron", "pairing"} {
		if got, ok := payload.Endpoints[key]; ok {
			t.Fatalf("unexpected root endpoint %q = %q", key, got)
		}
	}
}

func TestWriteJSONSuccessAndEncodeError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusCreated, map[string]any{"ok": true})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected json content type, got %q", got)
	}
	var payload map[string]bool
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload["ok"] {
		t.Fatalf("expected ok payload, got %v", payload)
	}

	recorder = httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, make(chan int))
	if !strings.Contains(recorder.Body.String(), "unsupported type") {
		t.Fatalf("expected encode error response, got %q", recorder.Body.String())
	}
}

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal int
		want       int
	}{
		{name: "empty", input: "", defaultVal: 10, want: 10},
		{name: "valid", input: "25", defaultVal: 10, want: 25},
		{name: "zero", input: "0", defaultVal: 10, want: 10},
		{name: "negative", input: "-1", defaultVal: 10, want: 10},
		{name: "invalid", input: "abc", defaultVal: 10, want: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseIntParam(tt.input, tt.defaultVal); got != tt.want {
				t.Fatalf("parseIntParam(%q, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}
