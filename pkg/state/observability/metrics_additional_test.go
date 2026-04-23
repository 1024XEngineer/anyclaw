package observability

import (
	"strings"
	"testing"
	"time"
)

func TestRegistryPrometheusFormatAndDefaultMetrics(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterDefaultMetrics()

	counter := registry.Counter("requests_total", "requests", map[string]string{"route": "/chat"})
	counter.Add(2)
	gauge := registry.Gauge("active_sessions", "sessions", map[string]string{"route": "/chat"})
	gauge.Set(3)
	histogram := registry.Histogram("duration_seconds", "duration", map[string]string{"route": "/chat"})
	histogram.Observe(0.2)

	output := registry.PrometheusFormat()
	for _, fragment := range []string{
		"# HELP requests_total requests",
		"# TYPE requests_total counter",
		`requests_total{route="/chat"} 2`,
		`active_sessions{route="/chat"} 3`,
		`duration_seconds_sum{route="/chat"} 0.2`,
		`duration_seconds_count{route="/chat"} 1`,
		`duration_seconds_bucket{le="0.250",route="/chat"} 1`,
		`duration_seconds_bucket{le="+Inf",route="/chat"} 1`,
		"anyclaw_requests_total",
		"anyclaw_goroutines",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected prometheus output to contain %q, got %s", fragment, output)
		}
	}
}

func TestMetricHelpersCloneAndPercentileEdgeCases(t *testing.T) {
	if got := cloneStringMap(nil); got != nil {
		t.Fatalf("expected nil string map clone, got %#v", got)
	}
	if got := cloneInt64Map(nil); got != nil {
		t.Fatalf("expected nil int64 map clone, got %#v", got)
	}

	clonedLabels := cloneStringMap(map[string]string{"route": "/chat"})
	clonedLabels["route"] = "/mutated"
	if got := cloneStringMap(map[string]string{"route": "/chat"})["route"]; got != "/chat" {
		t.Fatalf("expected cloned labels to be isolated, got %q", got)
	}

	clonedBuckets := cloneInt64Map(map[string]int64{"0.100": 1})
	clonedBuckets["0.100"] = 2
	if got := cloneInt64Map(map[string]int64{"0.100": 1})["0.100"]; got != 1 {
		t.Fatalf("expected cloned buckets to be isolated, got %d", got)
	}

	histogram := &Histogram{Buckets: make(map[string]int64)}
	if got := histogram.Percentile(0.95); got != 0 {
		t.Fatalf("expected empty histogram percentile 0, got %f", got)
	}

	histogram = &Histogram{
		Count:   1,
		Buckets: map[string]int64{"+Inf": 1},
	}
	if got := histogram.percentileLocked(0.99); got != 60 {
		t.Fatalf("expected fallback percentile 60, got %f", got)
	}

	labels := mergeLabels(map[string]string{"route": "/chat"}, map[string]string{"le": "0.100"})
	if labels["route"] != "/chat" || labels["le"] != "0.100" {
		t.Fatalf("unexpected merged labels: %#v", labels)
	}
}

func TestGaugeOperationsAndTimer(t *testing.T) {
	gauge := &Gauge{}
	gauge.Inc()
	gauge.Add(2)
	gauge.Dec()
	if got := gauge.Value; got != 2 {
		t.Fatalf("expected gauge value 2, got %f", got)
	}

	registry := NewRegistry()
	timer := registry.NewTimer("duration_seconds", "duration", nil)
	time.Sleep(5 * time.Millisecond)
	if got := timer.DurationMs(); got < 0 {
		t.Fatalf("expected non-negative timer duration, got %d", got)
	}
	timer.Stop()

	histogram := registry.Histogram("duration_seconds", "duration", nil)
	if histogram.Count != 1 {
		t.Fatalf("expected timer stop to record histogram observation, got count %d", histogram.Count)
	}
}
