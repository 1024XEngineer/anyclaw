package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPprofHandlers(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	PprofHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected pprof index to be served, got %d", rec.Code)
	}

	mux := http.NewServeMux()
	RegisterPprof(mux, "/custom/pprof")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/custom/pprof/", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected registered pprof index to be served, got %d", rec.Code)
	}
}
