package observability

import (
	"encoding/json"
	"net/http"
	stdruntime "runtime"
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

		sw := &gatewayHTTPStatusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sw, r)

		duration := time.Since(start)
		g.logger.Response(r.Method, r.URL.Path, sw.statusCode, duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)

		g.registry.Counter("anyclaw_requests_total", "Total HTTP requests", nil).Inc()
		if sw.statusCode >= 400 {
			g.registry.Counter("anyclaw_errors_total", "Total errors", nil).Inc()
		}
		g.registry.Histogram("anyclaw_request_duration_seconds", "HTTP request duration", nil).Observe(duration.Seconds())
	})
}

// TracingMiddleware records request traces.
func (g *GatewayHTTP) TracingMiddleware(next http.Handler) http.Handler {
	return TraceMiddleware(g.tp)(next)
}

// RegisterHealthChecks registers standard gateway health checks.
func (g *GatewayHTTP) RegisterHealthChecks(runtime GatewayHealthRuntime) {
	g.checker.Register("server", FuncCheck(func() error {
		return nil
	}))

	g.checker.Register("llm", TimeoutCheck(FuncCheck(func() error {
		if runtime == nil {
			return nil
		}
		name := runtime.LLMName()
		if name == "" {
			return nil
		}
		return nil
	}), 3*time.Second))

	g.checker.Register("memory", FuncCheck(func() error {
		if runtime == nil || !runtime.HasMemory() {
			return nil
		}
		return nil
	}))

	g.checker.SetDetails("server", map[string]any{
		"version":    g.version,
		"uptime":     time.Since(g.startTime).Round(time.Second).String(),
		"goroutines": stdruntime.NumGoroutine(),
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

type gatewayHTTPStatusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sw *gatewayHTTPStatusWriter) WriteHeader(code int) {
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}
