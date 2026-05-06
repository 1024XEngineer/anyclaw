package governance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

func TestWrapHandlesLocalCORSPreflight(t *testing.T) {
	called := false
	handler := Service{}.Wrap("/providers", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/providers", nil)
	req.Header.Set("Origin", "http://127.0.0.1:4173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("expected CORS preflight to skip wrapped handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /providers = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:4173" {
		t.Fatalf("unexpected allow origin %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("unexpected private network header %q", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("expected Vary to include Origin, got %q", got)
	}
}

func TestWrapLetsLocalCORSPreflightBypassAuth(t *testing.T) {
	called := false
	handler := Service{
		Auth: gatewayauth.NewMiddleware(&config.SecurityConfig{APIToken: "secret"}),
	}.Wrap("/providers", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/providers", nil)
	req.Header.Set("Origin", "http://localhost:4173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("expected authenticated route preflight to skip wrapped handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("authenticated OPTIONS /providers = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("expected preflight to bypass auth challenge, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:4173" {
		t.Fatalf("unexpected allow origin %q", got)
	}
}

func TestWrapAddsLocalCORSHeadersAndPassesThroughNonPreflight(t *testing.T) {
	handler := Service{}.Wrap("/providers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	req.Header.Set("Origin", "http://ui.localhost:4173")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("GET /providers = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://ui.localhost:4173" {
		t.Fatalf("unexpected allow origin %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPost) {
		t.Fatalf("expected allow methods to include POST, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Content-Type") {
		t.Fatalf("expected allow headers to include Content-Type, got %q", got)
	}
}

func TestWrapDoesNotAddCORSHeadersForRemoteOrigin(t *testing.T) {
	called := false
	handler := Service{}.Wrap("/providers", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/providers", nil)
	req.Header.Set("Origin", "https://example.com")
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected remote-origin OPTIONS request to reach wrapped handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin %q", got)
	}
}

func TestIsLocalControlOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{origin: "", want: false},
		{origin: ":", want: false},
		{origin: "null", want: true},
		{origin: "http://localhost:4173", want: true},
		{origin: "https://127.0.0.1", want: true},
		{origin: "wails://localhost", want: true},
		{origin: "http://[::1]:4173", want: true},
		{origin: "http://ui.localhost:4173", want: true},
		{origin: "ftp://localhost", want: false},
		{origin: "https://example.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			if got := isLocalControlOrigin(tt.origin); got != tt.want {
				t.Fatalf("isLocalControlOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}
