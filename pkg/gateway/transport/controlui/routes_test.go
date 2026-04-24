package controlui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterRoutesServesAssetsAndRedirects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, Options{BasePath: "/console", Root: root})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/console", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /console = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/console/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /console/app.js = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/market", nil))
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GET /market = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/discovery", nil))
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("HEAD /discovery = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got == "" {
		t.Fatal("expected redirect location")
	}

	h := routeHandler{opts: Options{BasePath: " ", Root: root}}
	if got := h.controlUIBasePath(); got != "/dashboard" {
		t.Fatalf("controlUIBasePath = %q", got)
	}
	if got := normalizeBasePath(" "); got != "/dashboard" {
		t.Fatalf("normalizeBasePath = %q", got)
	}
	if got := h.controlUIRoot(); got != root {
		t.Fatalf("controlUIRoot = %q", got)
	}
}
