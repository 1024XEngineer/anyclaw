package observability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubGatewayRuntime struct {
	llmName   string
	hasMemory bool
}

func (s stubGatewayRuntime) LLMName() string  { return s.llmName }
func (s stubGatewayRuntime) HasMemory() bool  { return s.hasMemory }

func TestGatewayHTTPHandlersAndMiddleware(t *testing.T) {
	gateway := NewGatewayHTTP("1.0.0")
	gateway.RegisterHealthChecks(stubGatewayRuntime{llmName: "gpt-test", hasMemory: true})

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("User-Agent", "test-agent")

	rec := httptest.NewRecorder()
	gateway.LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("unexpected response code from logging middleware: %d", rec.Code)
	}

	prom := gateway.registry.PrometheusFormat()
	for _, want := range []string{"anyclaw_requests_total", "anyclaw_errors_total", "anyclaw_request_duration_seconds_count"} {
		if !strings.Contains(prom, want) {
			t.Fatalf("expected %q in metrics output, got %s", want, prom)
		}
	}

	health := httptest.NewRecorder()
	gateway.HealthHandler().ServeHTTP(health, req)
	if health.Code != http.StatusOK {
		t.Fatalf("expected healthy gateway, got %d", health.Code)
	}

	ready := httptest.NewRecorder()
	gateway.ReadyHandler().ServeHTTP(ready, req)
	if ready.Code != http.StatusOK || !strings.Contains(ready.Body.String(), "ready") {
		t.Fatalf("unexpected readiness output: %d %s", ready.Code, ready.Body.String())
	}

	live := httptest.NewRecorder()
	gateway.LiveHandler().ServeHTTP(live, req)
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), "alive") {
		t.Fatalf("unexpected liveness output: %d %s", live.Code, live.Body.String())
	}

	metrics := httptest.NewRecorder()
	gateway.MetricsHandler().ServeHTTP(metrics, req)
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), "anyclaw_memory_usage_bytes") {
		t.Fatalf("unexpected Prometheus metrics output: %d %s", metrics.Code, metrics.Body.String())
	}

	metricsJSON := httptest.NewRecorder()
	gateway.MetricsJSONHandler().ServeHTTP(metricsJSON, req)
	if metricsJSON.Code != http.StatusOK {
		t.Fatalf("unexpected JSON metrics response: %d", metricsJSON.Code)
	}
	var decoded map[string]any
	if err := json.Unmarshal(metricsJSON.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("expected JSON metrics payload, got %v: %s", err, metricsJSON.Body.String())
	}
	if _, ok := decoded["gauges"]; !ok {
		t.Fatalf("expected gauges in JSON metrics, got %+v", decoded)
	}

	rec = httptest.NewRecorder()
	gateway.TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected response code from tracing middleware: %d", rec.Code)
	}

	writer := newObservabilityStatusWriter(httptest.NewRecorder())
	writer.WriteHeader(http.StatusCreated)
	if writer.StatusCode() != http.StatusCreated {
		t.Fatalf("expected status writer to record new code, got %d", writer.StatusCode())
	}
}
