package gateway

import (
	"encoding/json"
	"net/http"
	stdruntime "runtime"
	"time"

	"github.com/anyclaw/anyclaw/pkg/observability"
	"github.com/anyclaw/anyclaw/pkg/runtime"
)

// observabilityMiddleware wraps handlers with logging, tracing, and metrics.
type observabilityMiddleware struct {
	logger    *observability.Logger
	registry  *observability.Registry
	tp        *observability.TraceProvider
	checker   *observability.HealthChecker
	startTime time.Time
	version   string
}

func newObservabilityMiddleware(version string) *observabilityMiddleware {
	logger := observability.Global()
	registry := observability.NewRegistry()
	registry.RegisterDefaultMetrics()

	tp := observability.NewTraceProvider("anyclaw", observability.ConsoleExporter{})
	checker := observability.NewHealthChecker(version)

	return &observabilityMiddleware{
		logger:    logger,
		registry:  registry,
		tp:        tp,
		checker:   checker,
		startTime: time.Now(),
		version:   version,
	}
}

// LoggingMiddleware returns an HTTP middleware that logs requests with structured JSON.
func (om *observabilityMiddleware) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := &obsStatusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sw, r)

		duration := time.Since(start)
		om.logger.Response(r.Method, r.URL.Path, sw.statusCode, duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)

		om.registry.Counter("anyclaw_requests_total", "Total HTTP requests", nil).Inc()
		if sw.statusCode >= 400 {
			om.registry.Counter("anyclaw_errors_total", "Total errors", nil).Inc()
		}
		om.registry.Histogram("anyclaw_request_duration_seconds", "HTTP request duration", nil).Observe(duration.Seconds())
	})
}

// TracingMiddleware returns an HTTP middleware that traces requests.
func (om *observabilityMiddleware) TracingMiddleware(next http.Handler) http.Handler {
	return observability.TraceMiddleware(om.tp)(next)
}

// RegisterHealthChecks registers standard health checks.
func (om *observabilityMiddleware) RegisterHealthChecks(app *runtime.App) {
	om.checker.Register("server", observability.FuncCheck(func() error {
		return nil
	}))

	om.checker.Register("llm", observability.TimeoutCheck(observability.FuncCheck(func() error {
		if app == nil || app.LLM == nil {
			return nil
		}
		name := app.LLM.Name()
		if name == "" {
			return nil
		}
		return nil
	}), 3*time.Second))

	om.checker.Register("memory", observability.FuncCheck(func() error {
		if app == nil || app.Memory == nil {
			return nil
		}
		return nil
	}))

	om.checker.SetDetails("server", map[string]any{
		"version":    om.version,
		"uptime":     time.Since(om.startTime).Round(time.Second).String(),
		"goroutines": stdruntime.NumGoroutine(),
	})
}

// handleHealth serves the enhanced health check endpoint.
func (om *observabilityMiddleware) handleHealth(w http.ResponseWriter, r *http.Request) {
	om.checker.ServeHTTP(w, r)
}

// handleReady serves the readiness probe.
func (om *observabilityMiddleware) handleReady(w http.ResponseWriter, r *http.Request) {
	om.checker.ReadyHandler()(w, r)
}

// handleLive serves the liveness probe.
func (om *observabilityMiddleware) handleLive(w http.ResponseWriter, r *http.Request) {
	om.checker.LiveHandler()(w, r)
}

// handleMetrics serves Prometheus-format metrics.
func (om *observabilityMiddleware) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	om.registry.Gauge("anyclaw_memory_usage_bytes", "Memory usage in bytes", nil).Set(float64(mem.Alloc))
	om.registry.Gauge("anyclaw_goroutines", "Number of goroutines", nil).Set(float64(stdruntime.NumGoroutine()))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(om.registry.PrometheusFormat()))
}

// handleMetricsJSON serves metrics in JSON format.
func (om *observabilityMiddleware) handleMetricsJSON(w http.ResponseWriter, r *http.Request) {
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	om.registry.Gauge("anyclaw_memory_usage_bytes", "Memory usage in bytes", nil).Set(float64(mem.Alloc))
	om.registry.Gauge("anyclaw_goroutines", "Number of goroutines", nil).Set(float64(stdruntime.NumGoroutine()))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(om.registry.JSONFormat())
}

// handlePprof serves pprof endpoints.
func (om *observabilityMiddleware) handlePprof() http.Handler {
	return observability.PprofHandler()
}

type obsStatusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sw *obsStatusWriter) WriteHeader(code int) {
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}
