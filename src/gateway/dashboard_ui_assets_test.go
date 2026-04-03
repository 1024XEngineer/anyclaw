package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestHandleDashboardServesBuiltControlUIIndex(t *testing.T) {
	root := t.TempDir()
	uiRoot := filepath.Join(root, "dist", "control-ui")
	if err := os.MkdirAll(filepath.Join(uiRoot, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir ui root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiRoot, "index.html"), []byte("built-control-ui-index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", uiRoot)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "built-control-ui-index") {
		t.Fatalf("expected built index, got %q", body)
	}
}

func TestHandleDashboardServesStaticAssetUnderDashboard(t *testing.T) {
	root := t.TempDir()
	uiRoot := filepath.Join(root, "dist", "control-ui")
	if err := os.MkdirAll(filepath.Join(uiRoot, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir ui root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiRoot, "index.html"), []byte("built-control-ui-index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiRoot, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", uiRoot)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/dashboard/assets/app.js", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "console.log('ok')") {
		t.Fatalf("expected static asset, got %q", body)
	}
}

func TestHandleDashboardFallsBackToBuiltIndexForUnknownRoute(t *testing.T) {
	root := t.TempDir()
	uiRoot := filepath.Join(root, "dist", "control-ui")
	if err := os.MkdirAll(uiRoot, 0o755); err != nil {
		t.Fatalf("mkdir ui root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiRoot, "index.html"), []byte("spa-index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", uiRoot)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/dashboard/sessions/123", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "spa-index") {
		t.Fatalf("expected spa index fallback, got %q", body)
	}
}

func TestHandleDashboardFallsBackToEmbeddedDashboardWhenAssetsMissing(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "AnyClaw Console") {
		t.Fatalf("expected embedded dashboard fallback, got %q", body)
	}
}

func TestHandleDashboardSupportsConfiguredBasePath(t *testing.T) {
	root := t.TempDir()
	uiRoot := filepath.Join(root, "dist", "control-ui")
	if err := os.MkdirAll(uiRoot, 0o755); err != nil {
		t.Fatalf("mkdir ui root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiRoot, "index.html"), []byte("console-index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", uiRoot)

	cfg := config.DefaultConfig()
	cfg.Gateway.ControlUI.BasePath = "/console"
	s := &Server{
		app: &runtime.App{Config: cfg},
	}
	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "console-index") {
		t.Fatalf("expected configured-base dashboard index, got %q", body)
	}
}

func TestHandleDashboardSupportsLegacyControlRoute(t *testing.T) {
	root := t.TempDir()
	uiRoot := filepath.Join(root, "dist", "control-ui")
	if err := os.MkdirAll(uiRoot, 0o755); err != nil {
		t.Fatalf("mkdir ui root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiRoot, "index.html"), []byte("legacy-control-index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", uiRoot)

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/control", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "legacy-control-index") {
		t.Fatalf("expected legacy control index, got %q", body)
	}
}

func TestHandleRootAPIUsesConfiguredDashboardEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.ControlUI.BasePath = "/console"
	s := &Server{
		app: &runtime.App{Config: cfg},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.handleRootAPI(rec, req)

	var payload struct {
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode root response: %v", err)
	}
	if got := payload.Endpoints["dashboard"]; got != "/console" {
		t.Fatalf("expected dashboard endpoint /console, got %q", got)
	}
}
