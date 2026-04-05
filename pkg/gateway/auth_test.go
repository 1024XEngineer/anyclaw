package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestAuthMiddleware_NoToken(t *testing.T) {
	cfg := &config.SecurityConfig{
		APIToken: "",
		Users:    []config.SecurityUser{},
	}
	m := newAuthMiddleware(cfg)

	handler := m.Wrap("/status", func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			t.Fatal("expected user in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_WithToken_Valid(t *testing.T) {
	cfg := &config.SecurityConfig{
		APIToken: "secret-token",
		Users:    []config.SecurityUser{},
	}
	m := newAuthMiddleware(cfg)

	handler := m.Wrap("/status", func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			t.Fatal("expected user in context")
		}
		if user.Name != "admin" {
			t.Fatalf("expected admin user, got %s", user.Name)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_WithToken_Invalid(t *testing.T) {
	cfg := &config.SecurityConfig{
		APIToken: "secret-token",
		Users:    []config.SecurityUser{},
	}
	m := newAuthMiddleware(cfg)

	handler := m.Wrap("/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_PublicPath(t *testing.T) {
	cfg := &config.SecurityConfig{
		APIToken:    "secret-token",
		PublicPaths: []string{"/healthz", "/public"},
	}
	m := newAuthMiddleware(cfg)

	handler := m.Wrap("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for public path, got %d", w.Code)
	}
}

func TestAuthMiddleware_UserConfig(t *testing.T) {
	cfg := &config.SecurityConfig{
		APIToken: "",
		Users: []config.SecurityUser{
			{Name: "testuser", Token: "user-token", Role: "operator"},
		},
	}
	m := newAuthMiddleware(cfg)

	handler := m.Wrap("/status", func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			t.Fatal("expected user in context")
		}
		if user.Name != "testuser" {
			t.Fatalf("expected testuser, got %s", user.Name)
		}
		if user.Role != "operator" {
			t.Fatalf("expected operator role, got %s", user.Role)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_QueryToken(t *testing.T) {
	cfg := &config.SecurityConfig{
		APIToken: "secret-token",
	}
	m := newAuthMiddleware(cfg)

	handler := m.Wrap("/status", func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			t.Fatal("expected user in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/status?token=secret-token", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
