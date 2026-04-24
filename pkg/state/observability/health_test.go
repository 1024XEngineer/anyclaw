package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthCheckerHandlersAndChecks(t *testing.T) {
	hc := NewHealthChecker("1.2.3")
	hc.SetDetails("ok", map[string]any{"role": "primary"})
	hc.Register("ok", FuncCheck(func() error { return nil }))
	hc.Register("bad", FuncCheck(func() error { return errors.New("boom") }))

	report := hc.Check(context.Background())
	if report.Status != StatusUnhealthy {
		t.Fatalf("expected unhealthy report, got %+v", report)
	}
	if report.Version != "1.2.3" || report.Uptime == "" {
		t.Fatalf("expected version/uptime in report, got %+v", report)
	}
	if report.Components["ok"].Details["role"] != "primary" {
		t.Fatalf("expected component details to round-trip, got %+v", report.Components["ok"])
	}
	if hc.IsReady(context.Background()) {
		t.Fatal("expected readiness to fail when one component is unhealthy")
	}
	if !hc.IsLive() {
		t.Fatal("expected liveness to always be true")
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	hc.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unhealthy status code, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"unhealthy"`) {
		t.Fatalf("unexpected health response body: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	hc.ReadyHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "not_ready") {
		t.Fatalf("unexpected readiness response: %d %s", rec.Code, rec.Body.String())
	}

	live := httptest.NewRecorder()
	hc.LiveHandler().ServeHTTP(live, req)
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), "alive") {
		t.Fatalf("unexpected liveness response: %d %s", live.Code, live.Body.String())
	}
}

func TestHealthCheckHelpers(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer okServer.Close()

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badServer.Close()

	if err := HTTPCheck(okServer.URL, time.Second)(context.Background()); err != nil {
		t.Fatalf("expected healthy HTTP check, got %v", err)
	}
	if err := HTTPCheck(badServer.URL, time.Second)(context.Background()); err == nil {
		t.Fatal("expected 5xx HTTP check to fail")
	}
	if err := FuncCheck(func() error { return nil })(context.Background()); err != nil {
		t.Fatalf("FuncCheck: %v", err)
	}
	if err := TimeoutCheck(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, 10*time.Millisecond)(context.Background()); err == nil {
		t.Fatal("expected timeout-wrapped check to fail on context deadline")
	}
}
