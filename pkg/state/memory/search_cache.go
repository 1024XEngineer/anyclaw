package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

type SearchCache struct {
	mu        sync.RWMutex
	items     map[string]cacheEntry
	maxSize   int
	ttl       time.Duration
	hits      int64
	misses    int64
	evictions int64
}

type cacheEntry struct {
	results   []SearchResult
	expiresAt time.Time
	createdAt time.Time
}

type CacheConfig struct {
	MaxSize int
	TTL     time.Duration
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxSize: 5000,
		TTL:     5 * time.Minute,
	}
}

func NewSearchCache(cfg CacheConfig) *SearchCache {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 5000
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 5 * time.Minute
	}
	return &SearchCache{
		items:   make(map[string]cacheEntry),
		maxSize: cfg.MaxSize,
		ttl:     cfg.TTL,
	}
}

func (sc *SearchCache) Get(key string) ([]SearchResult, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	entry, ok := sc.items[key]
	if !ok {
		sc.misses++
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		sc.misses++
		return nil, false
	}
	sc.hits++
	return entry.results, true
}

func (sc *SearchCache) Set(key string, results []SearchResult) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if len(sc.items) >= sc.maxSize {
		sc.evictLocked()
	}

	sc.items[key] = cacheEntry{
		results:   results,
		expiresAt: time.Now().Add(sc.ttl),
		createdAt: time.Now(),
	}
}

func (sc *SearchCache) evictLocked() {
	now := time.Now()
	for k, v := range sc.items {
		if now.After(v.expiresAt) {
			delete(sc.items, k)
			sc.evictions++
		}
	}

	if len(sc.items) >= sc.maxSize {
		type kv struct {
			key string
			ts  time.Time
		}
		var oldest []kv
		for k, v := range sc.items {
			oldest = append(oldest, kv{k, v.createdAt})
		}
		sort.Slice(oldest, func(i, j int) bool {
			return oldest[i].ts.Before(oldest[j].ts)
		})

		half := len(oldest) / 2
		if half < 1 {
			half = 1
		}
		for i := 0; i < half; i++ {
			delete(sc.items, oldest[i].key)
			sc.evictions++
		}
	}
}

func (sc *SearchCache) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.items = make(map[string]cacheEntry)
}

func (sc *SearchCache) Len() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.items)
}

func (sc *SearchCache) Stats() CacheStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	total := sc.hits + sc.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(sc.hits) / float64(total)
	}

	return CacheStats{
		Size:      len(sc.items),
		MaxSize:   sc.maxSize,
		Hits:      sc.hits,
		Misses:    sc.misses,
		Evictions: sc.evictions,
		HitRate:   hitRate,
	}
}

type CacheStats struct {
	Size      int     `json:"size"`
	MaxSize   int     `json:"max_size"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	Evictions int64   `json:"evictions"`
	HitRate   float64 `json:"hit_rate"`
}

func (sc *SearchCache) ResetStats() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.hits = 0
	sc.misses = 0
	sc.evictions = 0
}

type WarmupConfig struct {
	Queries     []string
	Options     SearchOptions
	Concurrency int
	Entries     []MemoryEntry
	OnProgress  func(progress WarmupProgress)
}

type WarmupProgress struct {
	Total     int
	Processed int
	Failed    int
	Elapsed   time.Duration
	Current   string
	Message   string
	Done      bool
}

func WarmupCache(cache *SearchCache, cfg WarmupConfig) WarmupProgress {
	if len(cfg.Queries) == 0 {
		return WarmupProgress{Done: true}
	}

	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}

	start := time.Now()
	total := len(cfg.Queries)
	var processed int
	var failed int
	var mu sync.Mutex

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for _, query := range cfg.Queries {
		sem <- struct{}{}
		wg.Add(1)

		go func(q string) {
			defer wg.Done()
			defer func() { <-sem }()

			opts := cfg.Options
			opts.Context = context.Background()

			results := HybridSearch(cfg.Entries, q, opts)

			cacheKey := makeCacheKey(q, opts)
			cache.Set(cacheKey, results)

			mu.Lock()
			processed++
			p := processed
			mu.Unlock()

			if cfg.OnProgress != nil {
				cfg.OnProgress(WarmupProgress{
					Total:     total,
					Processed: p,
					Failed:    failed,
					Elapsed:   time.Since(start),
					Current:   q,
					Message:   "warmed: " + q,
				})
			}
		}(query)
	}

	wg.Wait()

	progress := WarmupProgress{
		Total:     total,
		Processed: processed,
		Failed:    failed,
		Elapsed:   time.Since(start),
		Done:      true,
		Message:   "warmup completed",
	}

	if cfg.OnProgress != nil {
		cfg.OnProgress(progress)
	}

	return progress
}

func makeCacheKey(query string, opts SearchOptions) string {
	h := sha256.New()
	h.Write([]byte(query))
	h.Write([]byte{byte(opts.Limit)})
	h.Write([]byte{byte(len(opts.Types))})
	for _, t := range opts.Types {
		h.Write([]byte(t))
	}
	if opts.ApplyTemporal {
		h.Write([]byte{1})
	}
	if opts.ApplyMMR {
		h.Write([]byte{2})
	}
	if opts.NormalizeScores {
		h.Write([]byte{3})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

type SearchCacheMiddleware struct {
	cache *SearchCache
}

func NewSearchCacheMiddleware(cache *SearchCache) *SearchCacheMiddleware {
	return &SearchCacheMiddleware{cache: cache}
}

func (m *SearchCacheMiddleware) Search(entries []MemoryEntry, query string, opts SearchOptions) []SearchResult {
	cacheKey := makeCacheKey(query, opts)

	if cached, ok := m.cache.Get(cacheKey); ok {
		return cached
	}

	results := HybridSearch(entries, query, opts)
	m.cache.Set(cacheKey, results)
	return results
}
