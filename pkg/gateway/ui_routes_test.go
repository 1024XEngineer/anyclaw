package gateway

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	controlui "github.com/anyclaw/anyclaw/pkg/gateway/transport/controlui"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestRegisterUIRoutesServesControlAndRedirectsLegacyPages(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "index.html"), "<html><body>control-ui</body></html>")
	mustWriteFile(t, filepath.Join(root, "assets", "app.js"), "console.log('control-ui');")

	cfg := config.DefaultConfig()
	cfg.Gateway.ControlUI.BasePath = "/dashboard"
	cfg.Gateway.ControlUI.Root = root

	server := &Server{
		mainRuntime: &appRuntime.App{Config: cfg},
	}

	mux := http.NewServeMux()
	controlui.RegisterRoutes(mux, controlui.Options{
		BasePath: server.mainRuntime.Config.Gateway.ControlUI.BasePath,
		Root:     server.mainRuntime.Config.Gateway.ControlUI.Root,
	})
	mux.HandleFunc("/", server.handleRootAPI)

	cases := []struct {
		path         string
		wantStatus   int
		wantBody     string
		wantLocation string
	}{
		{path: "/dashboard", wantStatus: http.StatusOK, wantBody: "control-ui"},
		{path: "/control", wantStatus: http.StatusOK, wantBody: "control-ui"},
		{path: "/dashboard/assets/app.js", wantStatus: http.StatusOK, wantBody: "console.log('control-ui');"},
		{path: "/market", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/dashboard#/market"},
		{path: "/discovery", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/dashboard#/discovery"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != tc.wantStatus {
			t.Fatalf("%s: expected %d, got %d", tc.path, tc.wantStatus, w.Code)
		}
		if tc.wantBody != "" && !strings.Contains(w.Body.String(), tc.wantBody) {
			t.Fatalf("%s: expected body to contain %q, got %q", tc.path, tc.wantBody, w.Body.String())
		}
		if tc.wantLocation != "" && w.Header().Get("Location") != tc.wantLocation {
			t.Fatalf("%s: expected redirect to %q, got %q", tc.path, tc.wantLocation, w.Header().Get("Location"))
		}
	}
}

func TestRegisterUIRoutesDoesNotShadowMarketAPI(t *testing.T) {
	cfg := config.DefaultConfig()
	server := &Server{
		mainRuntime: &appRuntime.App{Config: cfg},
	}

	mux := http.NewServeMux()
	controlui.RegisterRoutes(mux, controlui.Options{
		BasePath: server.mainRuntime.Config.Gateway.ControlUI.BasePath,
		Root:     server.mainRuntime.Config.Gateway.ControlUI.Root,
	})
	mux.HandleFunc("/market/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api-ok"))
	})
	mux.HandleFunc("/", server.handleRootAPI)

	req := httptest.NewRequest(http.MethodGet, "/market/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "api-ok" {
		t.Fatalf("expected market API handler to win, got %q", body)
	}
}

func TestRegisterUIRoutesRequiresBuiltControlUI(t *testing.T) {
	root := t.TempDir()

	mux := http.NewServeMux()
	controlui.RegisterRoutes(mux, controlui.Options{
		BasePath: "/dashboard",
		Root:     root,
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when control UI build is missing, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "control UI not found") {
		t.Fatalf("expected missing control UI message, got %q", w.Body.String())
	}
}

func mustWriteFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
