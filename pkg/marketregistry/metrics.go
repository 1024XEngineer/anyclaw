package marketregistry

import (
	"encoding/json"
	"net/http"
	stdruntime "runtime"
	"sync/atomic"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state/observability"
)

type registryMetrics struct {
	registry *observability.Registry

	queryCacheHits   atomic.Int64
	queryCacheMisses atomic.Int64
}

func newRegistryMetrics() *registryMetrics {
	reg := observability.NewRegistry()
	reg.Counter("anyclaw_registry_embedding_jobs_total", "Total embedding jobs processed", nil)
	reg.Counter("anyclaw_registry_embedding_jobs_failed_total", "Total failed embedding jobs", nil)
	reg.Counter("anyclaw_registry_vector_search_fallback_total", "Total vector-search fallback events", nil)
	reg.Counter("anyclaw_registry_vector_search_applied_total", "Total successful hybrid vector searches", nil)
	reg.Counter("anyclaw_registry_query_cache_hits_total", "Total query embedding cache hits", nil)
	reg.Counter("anyclaw_registry_query_cache_misses_total", "Total query embedding cache misses", nil)
	reg.Gauge("anyclaw_registry_query_cache_size", "Current query embedding cache size", nil)
	reg.Gauge("anyclaw_registry_embedding_jobs_queued", "Queued embedding jobs", nil)
	reg.Gauge("anyclaw_registry_embedding_jobs_failed", "Failed embedding jobs", nil)
	reg.Gauge("anyclaw_registry_artifact_embeddings_ready", "Ready artifact embeddings", nil)
	reg.Histogram("anyclaw_registry_query_embedding_duration_seconds", "Query embedding duration", nil)
	reg.Histogram("anyclaw_registry_vector_search_duration_seconds", "Vector search duration", nil)
	reg.Histogram("anyclaw_registry_embedding_job_duration_seconds", "Embedding job duration", nil)
	return &registryMetrics{registry: reg}
}

func (m *registryMetrics) recordQueryCacheHit() {
	if m == nil {
		return
	}
	m.queryCacheHits.Add(1)
	m.registry.Counter("anyclaw_registry_query_cache_hits_total", "Total query embedding cache hits", nil).Inc()
}

func (m *registryMetrics) recordQueryCacheMiss() {
	if m == nil {
		return
	}
	m.queryCacheMisses.Add(1)
	m.registry.Counter("anyclaw_registry_query_cache_misses_total", "Total query embedding cache misses", nil).Inc()
}

func (m *registryMetrics) recordVectorFallback(reason string) {
	if m == nil {
		return
	}
	m.registry.Counter("anyclaw_registry_vector_search_fallback_total", "Total vector-search fallback events", map[string]string{"reason": reason}).Inc()
}

func (m *registryMetrics) recordVectorApplied() {
	if m == nil {
		return
	}
	m.registry.Counter("anyclaw_registry_vector_search_applied_total", "Total successful hybrid vector searches", nil).Inc()
}

func (m *registryMetrics) newTimer(name, help string) *observability.Timer {
	if m == nil {
		return nil
	}
	return m.registry.NewTimer(name, help, nil)
}

func (m *registryMetrics) recordEmbeddingJob(success bool) {
	if m == nil {
		return
	}
	m.registry.Counter("anyclaw_registry_embedding_jobs_total", "Total embedding jobs processed", map[string]string{"status": boolLabel(success)}).Inc()
	if !success {
		m.registry.Counter("anyclaw_registry_embedding_jobs_failed_total", "Total failed embedding jobs", nil).Inc()
	}
}

func (m *registryMetrics) updateRuntimeState(cacheSize int, stats embeddingAdminStats) {
	if m == nil {
		return
	}
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	m.registry.Gauge("anyclaw_memory_usage_bytes", "Memory usage in bytes", nil).Set(float64(mem.Alloc))
	m.registry.Gauge("anyclaw_goroutines", "Number of goroutines", nil).Set(float64(stdruntime.NumGoroutine()))
	m.registry.Gauge("anyclaw_registry_query_cache_size", "Current query embedding cache size", nil).Set(float64(cacheSize))
	m.registry.Gauge("anyclaw_registry_embedding_jobs_queued", "Queued embedding jobs", nil).Set(float64(stats.QueuedJobs))
	m.registry.Gauge("anyclaw_registry_embedding_jobs_failed", "Failed embedding jobs", nil).Set(float64(stats.FailedJobs))
	m.registry.Gauge("anyclaw_registry_artifact_embeddings_ready", "Ready artifact embeddings", nil).Set(float64(stats.ReadyEmbeddings))
}

func (m *registryMetrics) prometheus(w http.ResponseWriter, r *http.Request) {
	if m == nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(m.registry.PrometheusFormat()))
}

func (m *registryMetrics) json(w http.ResponseWriter, r *http.Request) {
	if m == nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	payload := m.registry.JSONFormat()
	payload["registry_runtime"] = map[string]any{
		"query_cache_hits":   m.queryCacheHits.Load(),
		"query_cache_misses": m.queryCacheMisses.Load(),
		"generated_at":       time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func boolLabel(v bool) string {
	if v {
		return "success"
	}
	return "failed"
}
