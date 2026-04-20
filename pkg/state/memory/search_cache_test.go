package memory

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchCacheBasic(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())

	results := []SearchResult{{Entry: MemoryEntry{ID: "1"}, Score: 0.9}}
	cache.Set("query1", results)

	cached, ok := cache.Get("query1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(cached) != 1 || cached[0].Entry.ID != "1" {
		t.Errorf("unexpected cached result: %+v", cached)
	}
}

func TestSearchCacheMiss(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestSearchCacheTTL(t *testing.T) {
	cache := NewSearchCache(CacheConfig{
		MaxSize: 100,
		TTL:     50 * time.Millisecond,
	})

	cache.Set("ttl_test", []SearchResult{{Entry: MemoryEntry{ID: "1"}}})

	_, ok := cache.Get("ttl_test")
	if !ok {
		t.Fatal("expected cache hit before TTL")
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = cache.Get("ttl_test")
	if ok {
		t.Error("expected cache miss after TTL")
	}
}

func TestSearchCacheEviction(t *testing.T) {
	cache := NewSearchCache(CacheConfig{
		MaxSize: 5,
		TTL:     time.Hour,
	})

	for i := 0; i < 10; i++ {
		cache.Set(string(rune('a'+i)), []SearchResult{{Entry: MemoryEntry{ID: string(rune('a' + i))}}})
	}

	if cache.Len() > 5 {
		t.Errorf("expected cache size <= 5 after eviction, got %d", cache.Len())
	}
}

func TestSearchCacheClear(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())
	cache.Set("a", []SearchResult{{}})
	cache.Set("b", []SearchResult{{}})

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected empty cache after clear, got %d", cache.Len())
	}
}

func TestSearchCacheStats(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())

	cache.Set("q1", []SearchResult{{}})
	cache.Get("q1")
	cache.Get("q1")
	cache.Get("missing")

	stats := cache.Stats()

	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRate < 0.66 || stats.HitRate > 0.67 {
		t.Errorf("expected hit rate ~0.67, got %f", stats.HitRate)
	}
}

func TestSearchCacheResetStats(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())
	cache.Get("test")

	cache.ResetStats()

	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("expected reset stats to be 0, got hits=%d misses=%d", stats.Hits, stats.Misses)
	}
}

func TestMakeCacheKey(t *testing.T) {
	opts1 := SearchOptions{Limit: 10, UseKeyword: true}
	opts2 := SearchOptions{Limit: 10, UseKeyword: true}
	opts3 := SearchOptions{Limit: 20, UseKeyword: true}

	key1 := makeCacheKey("hello", opts1)
	key2 := makeCacheKey("hello", opts2)
	key3 := makeCacheKey("hello", opts3)

	if key1 != key2 {
		t.Error("expected same key for same options")
	}
	if key1 == key3 {
		t.Error("expected different key for different limit")
	}
}

func TestSearchCacheMiddleware(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())
	mw := NewSearchCacheMiddleware(cache)

	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Python programming", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.ApplyTemporal = false

	results1 := mw.Search(entries, "programming", opts)
	if len(results1) < 1 {
		t.Fatalf("expected results, got %d", len(results1))
	}

	stats := cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss on first search, got %d", stats.Misses)
	}

	results2 := mw.Search(entries, "programming", opts)
	if len(results2) != len(results1) {
		t.Errorf("expected same number of results from cache")
	}

	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit on second search, got %d", stats.Hits)
	}
}

func TestWarmupCache(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())

	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming language", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Python programming language", Timestamp: time.Now()},
		{ID: "3", Type: "fact", Content: "The weather is sunny", Timestamp: time.Now()},
	}

	queries := []string{"programming", "language", "weather"}

	var progressCount atomic.Int32
	cfg := WarmupConfig{
		Queries:     queries,
		Concurrency: 2,
		Entries:     entries,
		Options: SearchOptions{
			UseKeyword:    true,
			UseVector:     false,
			ApplyTemporal: false,
			Limit:         10,
		},
		OnProgress: func(p WarmupProgress) {
			progressCount.Add(1)
		},
	}

	progress := WarmupCache(cache, cfg)

	if !progress.Done {
		t.Error("expected warmup to be done")
	}
	if progress.Processed != 3 {
		t.Errorf("expected 3 processed, got %d", progress.Processed)
	}
	if cache.Len() != 3 {
		t.Errorf("expected 3 cached queries, got %d", cache.Len())
	}
	if progressCount.Load() == 0 {
		t.Error("expected progress callbacks")
	}

	for _, q := range queries {
		key := makeCacheKey(q, cfg.Options)
		_, ok := cache.Get(key)
		if !ok {
			t.Errorf("expected cache hit for query %q after warmup", q)
		}
	}
}

func TestWarmupCacheEmpty(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())

	progress := WarmupCache(cache, WarmupConfig{})

	if !progress.Done {
		t.Error("expected warmup to be done for empty config")
	}
}

func TestWarmupCacheWithConcurrency(t *testing.T) {
	cache := NewSearchCache(DefaultCacheConfig())

	entries := make([]MemoryEntry, 50)
	for i := range entries {
		entries[i] = MemoryEntry{
			ID:        string(rune('a' + i%26)),
			Type:      "fact",
			Content:   "test entry",
			Timestamp: time.Now(),
		}
	}

	queries := make([]string, 20)
	for i := range queries {
		queries[i] = "test"
	}

	cfg := WarmupConfig{
		Queries:     queries,
		Concurrency: 10,
		Entries:     entries,
		Options: SearchOptions{
			UseKeyword:    true,
			UseVector:     false,
			ApplyTemporal: false,
			Limit:         5,
		},
	}

	progress := WarmupCache(cache, cfg)

	if !progress.Done {
		t.Error("expected warmup to be done")
	}
	if progress.Processed != 20 {
		t.Errorf("expected 20 processed, got %d", progress.Processed)
	}
}
