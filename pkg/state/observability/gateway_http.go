package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	stdruntime "runtime"
	"strings"
	"time"
)

// GatewayHTTP exposes the HTTP-facing observability surface used by the gateway.
type GatewayHTTP struct {
	logger    *Logger
	registry  *Registry
	tp        *TraceProvider
	checker   *HealthChecker
	startTime time.Time
	version   string
}

type GatewayHealthRuntime interface {
	LLMName() string
	HasMemory() bool
}

type gatewayLLMAvailability interface {
	HasLLM() bool
}

type gatewayLLMHealthChecker interface {
	LLMHealthCheck(context.Context) error
}

type gatewayMemoryHealthChecker interface {
	MemoryHealthCheck(context.Context) error
}

// NewGatewayHTTP creates the gateway observability adapter.
func NewGatewayHTTP(version string) *GatewayHTTP {
	logger := Global()
	registry := NewRegistry()
	registry.RegisterDefaultMetrics()

	tp := NewTraceProvider("anyclaw", ConsoleExporter{})
	checker := NewHealthChecker(version)

	return &GatewayHTTP{
		logger:    logger,
		registry:  registry,
		tp:        tp,
		checker:   checker,
		startTime: time.Now(),
		version:   version,
	}
}

// LoggingMiddleware logs HTTP requests and records request metrics.
func (g *GatewayHTTP) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := newObservabilityStatusWriter(w)
		next.ServeHTTP(sw, r)

		duration := time.Since(start)
		g.logger.Response(r.Method, r.URL.Path, sw.StatusCode(), duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)

		g.registry.Counter("anyclaw_requests_total", "Total HTTP requests", nil).Inc()
		if sw.StatusCode() >= 400 {
			g.registry.Counter("anyclaw_errors_total", "Total errors", nil).Inc()
		}
		g.registry.Histogram("anyclaw_request_duration_seconds", "HTTP request duration", nil).Observe(duration.Seconds())
	})
}

// TracingMiddleware records request traces.
func (g *GatewayHTTP) TracingMiddleware(next http.Handler) http.Handler {
	return TraceMiddleware(g.tp)(next)
}

// Flush exports any buffered observability spans.
func (g *GatewayHTTP) Flush(ctx context.Context) error {
	if g == nil || g.tp == nil {
		return nil
	}
	return g.tp.Flush(ctx)
}

// Shutdown flushes and closes the gateway trace provider.
func (g *GatewayHTTP) Shutdown(ctx context.Context) error {
	if g == nil || g.tp == nil {
		return nil
	}
	return g.tp.Shutdown(ctx)
}

// RegisterHealthChecks registers standard gateway health checks.
func (g *GatewayHTTP) RegisterHealthChecks(runtime GatewayHealthRuntime) {
	g.checker.Register("server", FuncCheck(func() error {
		return nil
	}))

	g.checker.Register("llm", TimeoutCheck(func(ctx context.Context) error {
		return checkGatewayLLM(ctx, runtime)
	}, 3*time.Second))

	g.checker.Register("memory", TimeoutCheck(func(ctx context.Context) error {
		return checkGatewayMemory(ctx, runtime)
	}, 3*time.Second))

	g.checker.SetDetails("server", map[string]any{
		"version":    g.version,
		"uptime":     time.Since(g.startTime).Round(time.Second).String(),
		"goroutines": stdruntime.NumGoroutine(),
	})
	g.checker.SetDetails("llm", map[string]any{
		"name": configuredLLMName(runtime),
	})
	g.checker.SetDetails("memory", map[string]any{
		"enabled": runtime != nil && runtime.HasMemory(),
	})
}

// HealthHandler serves the enhanced health endpoint.
func (g *GatewayHTTP) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g.checker.ServeHTTP(w, r)
	}
}

// ReadyHandler serves the readiness probe.
func (g *GatewayHTTP) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g.checker.ReadyHandler()(w, r)
	}
}

// LiveHandler serves the liveness probe.
func (g *GatewayHTTP) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g.checker.LiveHandler()(w, r)
	}
}

// MetricsHandler serves Prometheus metrics.
func (g *GatewayHTTP) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem stdruntime.MemStats
		stdruntime.ReadMemStats(&mem)
		g.registry.Gauge("anyclaw_memory_usage_bytes", "Memory usage in bytes", nil).Set(float64(mem.Alloc))
		g.registry.Gauge("anyclaw_goroutines", "Number of goroutines", nil).Set(float64(stdruntime.NumGoroutine()))

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(g.registry.PrometheusFormat()))
	}
}

// MetricsJSONHandler serves metrics in JSON format.
func (g *GatewayHTTP) MetricsJSONHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem stdruntime.MemStats
		stdruntime.ReadMemStats(&mem)
		g.registry.Gauge("anyclaw_memory_usage_bytes", "Memory usage in bytes", nil).Set(float64(mem.Alloc))
		g.registry.Gauge("anyclaw_goroutines", "Number of goroutines", nil).Set(float64(stdruntime.NumGoroutine()))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(g.registry.JSONFormat())
	}
}

func checkGatewayLLM(ctx context.Context, runtime GatewayHealthRuntime) error {
	if runtime == nil {
		return fmt.Errorf("llm runtime is unavailable")
	}
	if checker, ok := runtime.(gatewayLLMHealthChecker); ok {
		return checker.LLMHealthCheck(ctx)
	}
	if availability, ok := runtime.(gatewayLLMAvailability); ok && !availability.HasLLM() {
		return fmt.Errorf("llm backend is unavailable")
	}
	if strings.TrimSpace(runtime.LLMName()) == "" {
		return fmt.Errorf("llm backend is unavailable")
	}
	return nil
}

func checkGatewayMemory(ctx context.Context, runtime GatewayHealthRuntime) error {
	if runtime == nil {
		return fmt.Errorf("memory runtime is unavailable")
	}
	if checker, ok := runtime.(gatewayMemoryHealthChecker); ok {
		return checker.MemoryHealthCheck(ctx)
	}
	if !runtime.HasMemory() {
		return fmt.Errorf("memory backend is unavailable")
	}
	return nil
}

func configuredLLMName(runtime GatewayHealthRuntime) string {
	if runtime == nil {
		return ""
	}
	return strings.TrimSpace(runtime.LLMName())
}
