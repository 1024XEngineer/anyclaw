package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health of a component.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusStarting  HealthStatus = "starting"
	StatusStopping  HealthStatus = "stopping"
)

// CheckFunc is a function that checks the health of a component.
type CheckFunc func(ctx context.Context) error

// Component represents a health-checked component.
type Component struct {
	Name      string         `json:"name"`
	Status    HealthStatus   `json:"status"`
	LatencyMs int64          `json:"latency_ms,omitempty"`
	Error     string         `json:"error,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// HealthReport is the overall health report.
type HealthReport struct {
	Status     HealthStatus          `json:"status"`
	Timestamp  time.Time             `json:"timestamp"`
	Version    string                `json:"version,omitempty"`
	Uptime     string                `json:"uptime,omitempty"`
	Components map[string]*Component `json:"components"`
}

// HealthChecker manages health checks for multiple components.
type HealthChecker struct {
	mu        sync.RWMutex
	checks    map[string]CheckFunc
	details   map[string]map[string]any
	startTime time.Time
	version   string
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(version string) *HealthChecker {
	return &HealthChecker{
		checks:    make(map[string]CheckFunc),
		details:   make(map[string]map[string]any),
		startTime: time.Now(),
		version:   version,
	}
}

// Register adds a health check for a component.
func (hc *HealthChecker) Register(name string, fn CheckFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks[name] = fn
}

// SetDetails sets static details for a component.
func (hc *HealthChecker) SetDetails(name string, details map[string]any) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.details[name] = details
}

// Check runs all health checks and returns a report.
func (hc *HealthChecker) Check(ctx context.Context) *HealthReport {
	hc.mu.RLock()
	checks := make(map[string]CheckFunc)
	details := make(map[string]map[string]any)
	for k, v := range hc.checks {
		checks[k] = v
	}
	for k, v := range hc.details {
		details[k] = v
	}
	hc.mu.RUnlock()

	report := &HealthReport{
		Status:     StatusHealthy,
		Timestamp:  time.Now().UTC(),
		Version:    hc.version,
		Uptime:     time.Since(hc.startTime).Round(time.Second).String(),
		Components: make(map[string]*Component),
	}

	for name, fn := range checks {
		start := time.Now()
		comp := &Component{
			Name:   name,
			Status: StatusHealthy,
		}

		if d, ok := details[name]; ok {
			comp.Details = d
		}

		if err := fn(ctx); err != nil {
			comp.Status = StatusUnhealthy
			comp.Error = err.Error()
			report.Status = StatusUnhealthy
		}

		comp.LatencyMs = time.Since(start).Milliseconds()
		report.Components[name] = comp
	}

	// If any component is unhealthy, overall is unhealthy
	// If any is degraded (no error but slow), overall is degraded
	if report.Status != StatusUnhealthy {
		for _, comp := range report.Components {
			if comp.Status == StatusDegraded {
				report.Status = StatusDegraded
				break
			}
		}
	}

	return report
}

// IsReady returns true if all checks pass.
func (hc *HealthChecker) IsReady(ctx context.Context) bool {
	report := hc.Check(ctx)
	return report.Status == StatusHealthy
}

// IsLive returns true if the process is running (basic liveness).
func (hc *HealthChecker) IsLive() bool {
	return true
}

// ServeHTTP implements http.Handler for the health endpoint.
func (hc *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	report := hc.Check(ctx)

	statusCode := http.StatusOK
	if report.Status == StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(report)
}

// ReadyHandler returns a simple readiness handler.
func (hc *HealthChecker) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if hc.IsReady(ctx) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
		}
	}
}

// LiveHandler returns a simple liveness handler.
func (hc *HealthChecker) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hc.IsLive() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"alive"}`))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"dead"}`))
		}
	}
}

// CheckFunc helpers

// HTTPCheck creates a check that verifies an HTTP endpoint responds.
func HTTPCheck(url string, timeout time.Duration) CheckFunc {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}
		return nil
	}
}

// FuncCheck creates a check from a simple function.
func FuncCheck(fn func() error) CheckFunc {
	return func(ctx context.Context) error {
		return fn()
	}
}

// TimeoutCheck wraps a check with a timeout.
func TimeoutCheck(fn CheckFunc, timeout time.Duration) CheckFunc {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return fn(ctx)
	}
}
