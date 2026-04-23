package observability

import (
	"strings"
	"testing"
	"time"
)

func TestRegistryFormatsAndHelpers(t *testing.T) {
	registry := NewRegistry()

	counter := registry.Counter("requests_total", "Total requests", map[string]string{"b": "2", "a": "1"})
	counter.Inc()
	counter.Add(2)
	if registry.Counter("requests_total", "ignored", map[string]string{"a": "1", "b": "2"}) != counter {
		t.Fatal("expected counter lookup with same labels to reuse the metric")
	}

	gauge := registry.Gauge("workers", "Workers", nil)
	gauge.Set(2)
	gauge.Inc()
	gauge.Dec()
	gauge.Add(1.5)

	histogram := registry.Histogram("latency_seconds", "Latency", map[string]string{"route": "/"})
	histogram.Observe(0.01)
	histogram.Observe(0.2)
	if histogram.Percentile(0.50) == 0 {
		t.Fatal("expected percentile to be computed")
	}

	timer := registry.NewTimer("timed_seconds", "Timed operation", nil)
	time.Sleep(2 * time.Millisecond)
	if timer.DurationMs() < 0 {
		t.Fatal("expected non-negative duration")
	}
	timer.Stop()

	prom := registry.PrometheusFormat()
	for _, want := range []string{"# HELP requests_total", "requests_total{a=\"1\",b=\"2\"} 3", "latency_seconds_count"} {
		if !strings.Contains(prom, want) {
			t.Fatalf("expected %q in Prometheus output, got %s", want, prom)
		}
	}

	jsonData := registry.JSONFormat()
	if len(jsonData["counters"].([]map[string]any)) == 0 || len(jsonData["gauges"].([]map[string]any)) == 0 || len(jsonData["histograms"].([]map[string]any)) == 0 {
		t.Fatalf("expected JSON metrics to include all metric kinds, got %+v", jsonData)
	}

	registry.RegisterDefaultMetrics()
	if len(registry.counters) == 0 || len(registry.gauges) == 0 || len(registry.latency) == 0 {
		t.Fatalf("expected default metrics to be registered, got %+v", registry)
	}

	if got := metricKey("metric", map[string]string{"b": "2", "a": "1"}); got != "metric{a=1,b=2}" {
		t.Fatalf("unexpected metric key: %q", got)
	}
	if got := labelsStr(map[string]string{"b": "2", "a": "1"}); got != "{a=\"1\",b=\"2\"}" {
		t.Fatalf("unexpected labels string: %q", got)
	}
	merged := mergeLabels(map[string]string{"a": "1"}, map[string]string{"b": "2"})
	if merged["a"] != "1" || merged["b"] != "2" {
		t.Fatalf("unexpected merged labels: %+v", merged)
	}

	emptyHistogram := &Histogram{Buckets: make(map[string]int64)}
	if emptyHistogram.Percentile(0.95) != 0 {
		t.Fatalf("expected empty histogram percentile to be zero, got %v", emptyHistogram.Percentile(0.95))
	}
}
