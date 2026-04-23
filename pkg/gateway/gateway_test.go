package gateway

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGatewayShellDefaults(t *testing.T) {
	runtimeRef := struct{ Name string }{Name: "main"}
	server := New(runtimeRef)
	if server == nil {
		t.Fatal("expected server")
	}
	if server.mainRuntime != runtimeRef {
		t.Fatalf("expected runtime reference to be retained")
	}
	if got := server.address(); got != defaultGatewayAddress {
		t.Fatalf("expected default address %q, got %q", defaultGatewayAddress, got)
	}
}

func TestGatewayAddressValidation(t *testing.T) {
	if got := (*Server)(nil).address(); got != defaultGatewayAddress {
		t.Fatalf("expected nil server to use default address, got %q", got)
	}
	if got := (&Server{addr: "not-a-host-port"}).address(); got != defaultGatewayAddress {
		t.Fatalf("expected invalid address to use default address, got %q", got)
	}
	if got := (&Server{addr: "127.0.0.1:0"}).address(); got != "127.0.0.1:0" {
		t.Fatalf("expected valid address to be preserved, got %q", got)
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

	server := &Server{addr: listener.Addr().String()}
	err = server.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gateway server failed") {
		t.Fatalf("expected listen failure, got %v", err)
	}
}

func TestGatewayRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := &Server{addr: "127.0.0.1:0"}
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
