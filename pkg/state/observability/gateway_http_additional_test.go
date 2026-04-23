package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fallbackGatewayRuntime struct {
	llmName   string
	hasLLM    bool
	hasMemory bool
}

func (r fallbackGatewayRuntime) LLMName() string { return r.llmName }
func (r fallbackGatewayRuntime) HasLLM() bool    { return r.hasLLM }
func (r fallbackGatewayRuntime) HasMemory() bool { return r.hasMemory }

type probedGatewayRuntime struct {
	fallbackGatewayRuntime
	llmErr    error
	memoryErr error
}

func (r probedGatewayRuntime) LLMHealthCheck(ctx context.Context) error {
	_ = ctx
	return r.llmErr
}

func (r probedGatewayRuntime) MemoryHealthCheck(ctx context.Context) error {
	_ = ctx
	return r.memoryErr
}

func TestGatewayHTTPMetricsHandlersAndNilLifecycle(t *testing.T) {
	var nilGateway *GatewayHTTP
	if err := nilGateway.Flush(context.Background()); err != nil {
		t.Fatalf("expected nil gateway flush to succeed, got %v", err)
	}
	if err := nilGateway.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected nil gateway shutdown to succeed, got %v", err)
	}

	gateway := NewGatewayHTTP("test")
	handler := gateway.LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/chat", nil))

	metricsRec := httptest.NewRecorder()
	gateway.MetricsHandler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics handler to return 200, got %d", metricsRec.Code)
	}
	if got := metricsRec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("expected prometheus content type, got %q", got)
	}
	if body := metricsRec.Body.String(); !strings.Contains(body, "anyclaw_requests_total") || !strings.Contains(body, "anyclaw_errors_total") {
		t.Fatalf("expected prometheus metrics body, got %s", body)
	}

	jsonRec := httptest.NewRecorder()
	gateway.MetricsJSONHandler().ServeHTTP(jsonRec, httptest.NewRequest(http.MethodGet, "/metrics.json", nil))
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("expected metrics json handler to return 200, got %d", jsonRec.Code)
	}
	if got := jsonRec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected json content type, got %q", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode metrics json: %v", err)
	}
	if len(payload["counters"].([]any)) == 0 {
		t.Fatal("expected counters in metrics json payload")
	}

	liveRec := httptest.NewRecorder()
	gateway.LiveHandler().ServeHTTP(liveRec, httptest.NewRequest(http.MethodGet, "/live", nil))
	if liveRec.Code != http.StatusOK {
		t.Fatalf("expected gateway live handler to return 200, got %d", liveRec.Code)
	}

	traceExporter := &recordingExporter{}
	gateway.tp = NewTraceProvider("svc", traceExporter)
	traced := gateway.TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	traced.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/trace", nil))
	if traceExporter.exportedCount() != 1 {
		t.Fatalf("expected tracing middleware to export a span, got %d", traceExporter.exportedCount())
	}

	if err := gateway.Flush(context.Background()); err != nil {
		t.Fatalf("expected gateway flush to succeed, got %v", err)
	}
}

func TestGatewayHealthCheckHelpers(t *testing.T) {
	if err := checkGatewayLLM(context.Background(), nil); err == nil || err.Error() != "llm runtime is unavailable" {
		t.Fatalf("expected nil llm runtime error, got %v", err)
	}
	if err := checkGatewayMemory(context.Background(), nil); err == nil || err.Error() != "memory runtime is unavailable" {
		t.Fatalf("expected nil memory runtime error, got %v", err)
	}

	if got := configuredLLMName(fallbackGatewayRuntime{llmName: " openai "}); got != "openai" {
		t.Fatalf("expected trimmed llm name, got %q", got)
	}
	if got := configuredLLMName(nil); got != "" {
		t.Fatalf("expected empty llm name for nil runtime, got %q", got)
	}

	unavailableLLM := fallbackGatewayRuntime{llmName: "openai", hasLLM: false, hasMemory: true}
	if err := checkGatewayLLM(context.Background(), unavailableLLM); err == nil || err.Error() != "llm backend is unavailable" {
		t.Fatalf("expected llm unavailable error, got %v", err)
	}

	blankLLM := fallbackGatewayRuntime{llmName: " ", hasLLM: true, hasMemory: true}
	if err := checkGatewayLLM(context.Background(), blankLLM); err == nil || err.Error() != "llm backend is unavailable" {
		t.Fatalf("expected blank llm name error, got %v", err)
	}

	unavailableMemory := fallbackGatewayRuntime{llmName: "openai", hasLLM: true, hasMemory: false}
	if err := checkGatewayMemory(context.Background(), unavailableMemory); err == nil || err.Error() != "memory backend is unavailable" {
		t.Fatalf("expected memory unavailable error, got %v", err)
	}

	runtime := probedGatewayRuntime{
		fallbackGatewayRuntime: fallbackGatewayRuntime{
			llmName:   "openai",
			hasLLM:    true,
			hasMemory: true,
		},
		llmErr:    errors.New("probe failed"),
		memoryErr: errors.New("memory failed"),
	}
	if err := checkGatewayLLM(context.Background(), runtime); err == nil || err.Error() != "probe failed" {
		t.Fatalf("expected llm probe failure, got %v", err)
	}
	if err := checkGatewayMemory(context.Background(), runtime); err == nil || err.Error() != "memory failed" {
		t.Fatalf("expected memory probe failure, got %v", err)
	}

	healthyRuntime := fallbackGatewayRuntime{llmName: "openai", hasLLM: true, hasMemory: true}
	if err := checkGatewayLLM(context.Background(), healthyRuntime); err != nil {
		t.Fatalf("expected healthy llm runtime, got %v", err)
	}
	if err := checkGatewayMemory(context.Background(), healthyRuntime); err != nil {
		t.Fatalf("expected healthy memory runtime, got %v", err)
	}
}
