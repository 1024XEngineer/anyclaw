package observability

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry holds all metrics.
type Registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
	latency  map[string]*Histogram
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*Gauge),
		latency:  make(map[string]*Histogram),
	}
}

// Counter is a monotonically increasing metric.
type Counter struct {
	Name   string            `json:"name"`
	Help   string            `json:"help"`
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
	mu     sync.Mutex
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Value++
}

// Add adds a value to the counter.
func (c *Counter) Add(v float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Value += v
}

// Gauge is a metric that can go up and down.
type Gauge struct {
	Name   string            `json:"name"`
	Help   string            `json:"help"`
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
	mu     sync.Mutex
}

// Set sets the gauge value.
func (g *Gauge) Set(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Value = v
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Value++
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Value--
}

// Add adds a value to the gauge.
func (g *Gauge) Add(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Value += v
}

// Histogram tracks value distributions.
type Histogram struct {
	Name    string            `json:"name"`
	Help    string            `json:"help"`
	Labels  map[string]string `json:"labels"`
	Count   int64             `json:"count"`
	Sum     float64           `json:"sum"`
	Min     float64           `json:"min"`
	Max     float64           `json:"max"`
	Buckets map[string]int64  `json:"buckets"`
	mu      sync.Mutex
}

// Observe records a value.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Count++
	h.Sum += v
	if h.Count == 1 || v < h.Min {
		h.Min = v
	}
	if h.Count == 1 || v > h.Max {
		h.Max = v
	}

	boundaries := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
	for _, b := range boundaries {
		label := fmt.Sprintf("%.3f", b)
		if v <= b {
			h.Buckets[label]++
		}
	}
	h.Buckets["+Inf"]++
}

// Percentile calculates an approximate percentile.
func (h *Histogram) Percentile(p float64) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Count == 0 {
		return 0
	}
	target := int64(math.Ceil(float64(h.Count) * p))
	boundaries := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
	for _, b := range boundaries {
		label := fmt.Sprintf("%.3f", b)
		if h.Buckets[label] >= target {
			return b
		}
	}
	return 60
}

// Counter creates or gets a counter.
func (r *Registry) Counter(name, help string, labels map[string]string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := metricKey(name, labels)
	if c, ok := r.counters[key]; ok {
		return c
	}

	c := &Counter{
		Name:   name,
		Help:   help,
		Labels: labels,
	}
	r.counters[key] = c
	return c
}

// Gauge creates or gets a gauge.
func (r *Registry) Gauge(name, help string, labels map[string]string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := metricKey(name, labels)
	if g, ok := r.gauges[key]; ok {
		return g
	}

	g := &Gauge{
		Name:   name,
		Help:   help,
		Labels: labels,
	}
	r.gauges[key] = g
	return g
}

// Histogram creates or gets a histogram.
func (r *Registry) Histogram(name, help string, labels map[string]string) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := metricKey(name, labels)
	if h, ok := r.latency[key]; ok {
		return h
	}

	h := &Histogram{
		Name:    name,
		Help:    help,
		Labels:  labels,
		Min:     math.MaxFloat64,
		Buckets: make(map[string]int64),
	}
	r.latency[key] = h
	return h
}

// Timer is a convenience type for timing operations.
type Timer struct {
	histogram *Histogram
	start     time.Time
}

// NewTimer creates a new timer.
func (r *Registry) NewTimer(name, help string, labels map[string]string) *Timer {
	return &Timer{
		histogram: r.Histogram(name, help, labels),
		start:     time.Now(),
	}
}

// Stop records the elapsed time.
func (t *Timer) Stop() {
	t.histogram.Observe(time.Since(t.start).Seconds())
}

// DurationMs returns the elapsed time in milliseconds.
func (t *Timer) DurationMs() int64 {
	return time.Since(t.start).Milliseconds()
}

// PrometheusFormat returns metrics in Prometheus text exposition format.
func (r *Registry) PrometheusFormat() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var sb strings.Builder

	// Counters
	for _, c := range sortedCounters(r.counters) {
		c.mu.Lock()
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", c.Name, c.Help))
		sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", c.Name))
		sb.WriteString(fmt.Sprintf("%s%s %.0f\n", c.Name, labelsStr(c.Labels), c.Value))
		c.mu.Unlock()
	}

	// Gauges
	for _, g := range sortedGauges(r.gauges) {
		g.mu.Lock()
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", g.Name, g.Help))
		sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", g.Name))
		sb.WriteString(fmt.Sprintf("%s%s %g\n", g.Name, labelsStr(g.Labels), g.Value))
		g.mu.Unlock()
	}

	// Histograms
	for _, h := range sortedHistograms(r.latency) {
		h.mu.Lock()
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", h.Name, h.Help))
		sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", h.Name))
		sb.WriteString(fmt.Sprintf("%s_sum%s %g\n", h.Name, labelsStr(h.Labels), h.Sum))
		sb.WriteString(fmt.Sprintf("%s_count%s %d\n", h.Name, labelsStr(h.Labels), h.Count))

		boundaries := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
		for _, b := range boundaries {
			label := fmt.Sprintf("%.3f", b)
			merged := mergeLabels(h.Labels, map[string]string{"le": label})
			sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.Name, labelsStr(merged), h.Buckets[label]))
		}
		merged := mergeLabels(h.Labels, map[string]string{"le": "+Inf"})
		sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.Name, labelsStr(merged), h.Buckets["+Inf"]))
		h.mu.Unlock()
	}

	return sb.String()
}

// JSONFormat returns metrics as JSON-serializable maps.
func (r *Registry) JSONFormat() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := map[string]any{
		"counters":   make([]map[string]any, 0),
		"gauges":     make([]map[string]any, 0),
		"histograms": make([]map[string]any, 0),
	}

	counters := result["counters"].([]map[string]any)
	for _, c := range r.counters {
		c.mu.Lock()
		counters = append(counters, map[string]any{
			"name":   c.Name,
			"help":   c.Help,
			"labels": c.Labels,
			"value":  c.Value,
		})
		c.mu.Unlock()
	}

	gauges := result["gauges"].([]map[string]any)
	for _, g := range r.gauges {
		g.mu.Lock()
		gauges = append(gauges, map[string]any{
			"name":   g.Name,
			"help":   g.Help,
			"labels": g.Labels,
			"value":  g.Value,
		})
		g.mu.Unlock()
	}

	histograms := result["histograms"].([]map[string]any)
	for _, h := range r.latency {
		h.mu.Lock()
		histograms = append(histograms, map[string]any{
			"name":    h.Name,
			"help":    h.Help,
			"labels":  h.Labels,
			"count":   h.Count,
			"sum":     h.Sum,
			"min":     h.Min,
			"max":     h.Max,
			"p50":     h.Percentile(0.50),
			"p95":     h.Percentile(0.95),
			"p99":     h.Percentile(0.99),
			"buckets": h.Buckets,
		})
		h.mu.Unlock()
	}

	return result
}

// RegisterDefaultMetrics registers standard application metrics.
func (r *Registry) RegisterDefaultMetrics() {
	r.Counter("anyclaw_requests_total", "Total HTTP requests", map[string]string{})
	r.Counter("anyclaw_errors_total", "Total errors", map[string]string{})
	r.Gauge("anyclaw_active_sessions", "Number of active sessions", map[string]string{})
	r.Gauge("anyclaw_active_agents", "Number of active agents", map[string]string{})
	r.Histogram("anyclaw_request_duration_seconds", "HTTP request duration", map[string]string{})
	r.Histogram("anyclaw_llm_call_duration_seconds", "LLM call duration", map[string]string{})
	r.Histogram("anyclaw_tool_call_duration_seconds", "Tool call duration", map[string]string{})
	r.Counter("anyclaw_tool_calls_total", "Total tool calls", map[string]string{})
	r.Counter("anyclaw_llm_calls_total", "Total LLM API calls", map[string]string{})
	r.Counter("anyclaw_workflow_executions_total", "Total workflow executions", map[string]string{})
	r.Gauge("anyclaw_memory_usage_bytes", "Memory usage in bytes", map[string]string{})
	r.Gauge("anyclaw_goroutines", "Number of goroutines", map[string]string{})
}

func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

func labelsStr(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(labels))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func mergeLabels(a, b map[string]string) map[string]string {
	m := make(map[string]string)
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}

func sortedCounters(m map[string]*Counter) []*Counter {
	result := make([]*Counter, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func sortedGauges(m map[string]*Gauge) []*Gauge {
	result := make([]*Gauge, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func sortedHistograms(m map[string]*Histogram) []*Histogram {
	result := make([]*Histogram, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
