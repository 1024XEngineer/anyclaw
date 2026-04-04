package media

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMediaCache_SetAndGet(t *testing.T) {
	cfg := DefaultMediaCacheConfig()
	cache := NewMediaCache(cfg)

	media := &Media{
		ID:       "test-1",
		Type:     TypeImage,
		MimeType: "image/png",
		Size:     1024,
		Data:     make([]byte, 1024),
		URL:      "https://example.com/image.png",
	}

	key := MakeMediaCacheKey("https://example.com/image.png")
	cache.Set(key, media)

	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != "test-1" {
		t.Errorf("expected ID test-1, got %s", got.ID)
	}
	if got.Size != 1024 {
		t.Errorf("expected size 1024, got %d", got.Size)
	}
}

func TestMediaCache_Miss(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	_, ok := cache.Get("nonexistent-key")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestMediaCache_TTLExpiration(t *testing.T) {
	cfg := MediaCacheConfig{
		MaxItems: 100,
		MaxBytes: 10 * 1024 * 1024,
		TTL:      100 * time.Millisecond,
	}
	cache := NewMediaCache(cfg)

	media := &Media{
		ID:   "test-ttl",
		Size: 512,
		Data: make([]byte, 512),
	}

	key := MakeMediaCacheKey("https://example.com/expiring")
	cache.Set(key, media)

	_, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit before expiration")
	}

	time.Sleep(150 * time.Millisecond)

	_, ok = cache.Get(key)
	if ok {
		t.Error("expected cache miss after TTL expiration")
	}
}

func TestMediaCache_Eviction(t *testing.T) {
	cfg := MediaCacheConfig{
		MaxItems: 3,
		MaxBytes: 10 * 1024 * 1024,
		TTL:      1 * time.Hour,
	}
	cache := NewMediaCache(cfg)

	for i := 0; i < 5; i++ {
		media := &Media{
			ID:   fmt.Sprintf("item-%d", i),
			Size: 1024,
			Data: make([]byte, 1024),
		}
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, media)
	}

	if cache.Len() > 3 {
		t.Errorf("expected at most 3 items after eviction, got %d", cache.Len())
	}
}

func TestMediaCache_Remove(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	media := &Media{
		ID:   "test-remove",
		Size: 256,
		Data: make([]byte, 256),
	}
	key := "remove-key"
	cache.Set(key, media)

	if cache.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", cache.Len())
	}

	cache.Remove(key)

	if cache.Len() != 0 {
		t.Errorf("expected 0 items after remove, got %d", cache.Len())
	}
}

func TestMediaCache_Clear(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	for i := 0; i < 10; i++ {
		media := &Media{
			ID:   fmt.Sprintf("item-%d", i),
			Size: 100,
			Data: make([]byte, 100),
		}
		cache.Set(fmt.Sprintf("key-%d", i), media)
	}

	if cache.Len() != 10 {
		t.Fatalf("expected 10 items, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected 0 items after clear, got %d", cache.Len())
	}
	if cache.SizeBytes() != 0 {
		t.Errorf("expected 0 bytes after clear, got %d", cache.SizeBytes())
	}
}

func TestMediaCache_SizeBytes(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	media := &Media{
		ID:   "test-size",
		Size: 2048,
		Data: make([]byte, 2048),
	}
	cache.Set("size-key", media)

	if cache.SizeBytes() != 2048 {
		t.Errorf("expected 2048 bytes, got %d", cache.SizeBytes())
	}
}

func TestMediaCache_Stats(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	media := &Media{
		ID:   "test-stats",
		Size: 512,
		Data: make([]byte, 512),
	}
	cache.Set("stats-key", media)

	cache.Get("stats-key")
	cache.Get("stats-key")
	cache.Get("nonexistent")

	stats := cache.Stats()

	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.ItemCount != 1 {
		t.Errorf("expected 1 item, got %d", stats.ItemCount)
	}
}

func TestMediaCache_Cleanup(t *testing.T) {
	cfg := MediaCacheConfig{
		MaxItems: 100,
		MaxBytes: 10 * 1024 * 1024,
		TTL:      50 * time.Millisecond,
	}
	cache := NewMediaCache(cfg)

	for i := 0; i < 5; i++ {
		media := &Media{
			ID:   fmt.Sprintf("item-%d", i),
			Size: 100,
			Data: make([]byte, 100),
		}
		cache.Set(fmt.Sprintf("key-%d", i), media)
	}

	time.Sleep(100 * time.Millisecond)

	removed := cache.Cleanup()

	if removed != 5 {
		t.Errorf("expected 5 expired items cleaned up, got %d", removed)
	}
	if cache.Len() != 0 {
		t.Errorf("expected 0 items after cleanup, got %d", cache.Len())
	}
}

func TestMediaCache_DiskPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "media-cache")

	cfg := MediaCacheConfig{
		MaxItems:     100,
		MaxBytes:     10 * 1024 * 1024,
		TTL:          1 * time.Hour,
		DiskPath:     cacheDir,
		PersistIndex: true,
	}
	cache := NewMediaCache(cfg)

	data := []byte("test image data for disk persistence")
	media := &Media{
		ID:       "disk-test",
		Type:     TypeImage,
		MimeType: "image/png",
		Size:     int64(len(data)),
		Data:     data,
		URL:      "https://example.com/disk.png",
	}

	key := MakeMediaCacheKey("https://example.com/disk.png")
	cache.Set(key, media)

	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(got.Data) != string(data) {
		t.Error("data mismatch")
	}

	indexPath := filepath.Join(cacheDir, "cache_index.json")
	if _, err := os.Stat(indexPath); err != nil {
		t.Errorf("expected cache index file to exist: %v", err)
	}
}

func TestMediaCache_MaxBytes(t *testing.T) {
	cfg := MediaCacheConfig{
		MaxItems: 1000,
		MaxBytes: 2048,
		TTL:      1 * time.Hour,
	}
	cache := NewMediaCache(cfg)

	largeMedia := &Media{
		ID:   "large",
		Size: 3000,
		Data: make([]byte, 3000),
	}
	cache.Set("large-key", largeMedia)

	if cache.Len() != 0 {
		t.Error("expected large item to be rejected")
	}
}

func TestMediaCache_NilSet(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())
	cache.Set("nil-key", nil)

	if cache.Len() != 0 {
		t.Error("expected nil media to be ignored")
	}
}

func TestMediaCache_ConcurrentAccess(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := fmt.Sprintf("concurrent-%d", idx)
			media := &Media{
				ID:   key,
				Size: 100,
				Data: make([]byte, 100),
			}
			cache.Set(key, media)

			cache.Get(key)
		}(i)
	}

	wg.Wait()

	if cache.Len() != 50 {
		t.Errorf("expected 50 items, got %d", cache.Len())
	}
}

func TestMediaCache_HitRate(t *testing.T) {
	cache := NewMediaCache(DefaultMediaCacheConfig())

	media := &Media{
		ID:   "hitrate",
		Size: 100,
		Data: make([]byte, 100),
	}
	cache.Set("hitrate-key", media)

	cache.Get("hitrate-key")
	cache.Get("hitrate-key")
	cache.Get("miss-key")
	cache.Get("miss-key-2")

	stats := cache.Stats()

	if stats.LastHitRate == 0 {
		t.Error("expected non-zero hit rate")
	}
}

func TestMakeMediaCacheKey(t *testing.T) {
	key1 := MakeMediaCacheKey("https://example.com/image.png")
	key2 := MakeMediaCacheKey("https://example.com/image.png")
	key3 := MakeMediaCacheKey("https://example.com/different.png")

	if key1 != key2 {
		t.Error("same URL should produce same key")
	}
	if key1 == key3 {
		t.Error("different URLs should produce different keys")
	}
}

func TestMediaCacheOptions(t *testing.T) {
	key := MakeMediaCacheKey("https://example.com/test.png",
		WithCacheMaxSize(5*1024*1024),
		WithCacheFormat("png"),
	)

	if key == "" {
		t.Error("expected non-empty key")
	}
}

func TestMediaPipeline_Download(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("fake-data"))
	}))
	defer server.Close()

	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	media, err := pipeline.Download(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if media == nil {
		t.Fatal("expected media result")
	}
	if media.Type != TypeDoc {
		t.Errorf("expected doc type, got %s", media.Type)
	}
	if media.MimeType != "text/plain" {
		t.Errorf("expected text/plain, got %s", media.MimeType)
	}
}

func TestMediaPipeline_DownloadWithCache(t *testing.T) {
	callCount := int32(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("jpeg-data"))
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.CacheConfig = MediaCacheConfig{
		MaxItems: 100,
		MaxBytes: 10 * 1024 * 1024,
		TTL:      1 * time.Hour,
	}

	pipeline := NewMediaPipeline(cfg)

	_, err := pipeline.Download(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("first download failed: %v", err)
	}

	_, err = pipeline.Download(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("second download failed: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 HTTP call (second should be cached), got %d", callCount)
	}

	stats := pipeline.Stats()
	if stats.DownloadsCached != 1 {
		t.Errorf("expected 1 cached download, got %d", stats.DownloadsCached)
	}
}

func TestMediaPipeline_DownloadEmptyURL(t *testing.T) {
	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	_, err := pipeline.Download(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestMediaPipeline_DownloadBlockedScheme(t *testing.T) {
	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	_, err := pipeline.Download(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Error("expected error for file:// scheme")
	}
}

func TestMediaPipeline_DownloadFailAndRetry(t *testing.T) {
	attempts := int32(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("success-after-retry"))
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.RetryCount = 3
	cfg.RetryDelay = 10 * time.Millisecond
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	media, err := pipeline.Download(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("expected success after retry: %v", err)
	}

	if string(media.Data) != "success-after-retry" {
		t.Errorf("expected success data, got %s", string(media.Data))
	}
}

func TestMediaPipeline_DownloadAllRetriesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.RetryCount = 2
	cfg.RetryDelay = 10 * time.Millisecond
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	_, err := pipeline.Download(context.Background(), server.URL)
	if err == nil {
		t.Error("expected error after all retries")
	}

	stats := pipeline.Stats()
	if stats.DownloadsFailed != 1 {
		t.Errorf("expected 1 failed download, got %d", stats.DownloadsFailed)
	}
}

func TestMediaPipeline_DownloadBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("batch-image"))
	}))
	defer server.Close()

	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	urls := []string{
		server.URL + "/1",
		server.URL + "/2",
		server.URL + "/3",
	}

	results := pipeline.DownloadBatch(context.Background(), urls)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for i, result := range results {
		if result.Error != nil {
			t.Errorf("result %d had error: %v", i, result.Error)
		}
		if result.Media == nil {
			t.Errorf("result %d had nil media", i)
		}
	}
}

func TestMediaPipeline_DownloadBatchWithFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("image"))
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.RetryCount = 0
	cfg.RetryDelay = 0
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	urls := []string{
		server.URL + "/ok1",
		server.URL + "/fail",
		server.URL + "/ok2",
	}

	results := pipeline.DownloadBatch(context.Background(), urls)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].Error != nil {
		t.Errorf("first should succeed: %v", results[0].Error)
	}
	if results[1].Error == nil {
		t.Error("second should fail")
	}
	if results[2].Error != nil {
		t.Errorf("third should succeed: %v", results[2].Error)
	}
}

func TestMediaPipeline_Hooks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("hook-test"))
	}))
	defer server.Close()

	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	var beforeCalled, afterCalled bool
	var downloadedMedia *Media
	var wasCached bool

	hook := &testHook{
		beforeFn: func(ctx context.Context, url string, req *MediaDownloadRequest) error {
			beforeCalled = true
			return nil
		},
		afterFn: func(ctx context.Context, media *Media, cached bool) error {
			afterCalled = true
			downloadedMedia = media
			wasCached = cached
			return nil
		},
	}

	pipeline.RegisterHook(hook)

	_, err := pipeline.Download(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if !beforeCalled {
		t.Error("OnBeforeDownload hook not called")
	}
	if !afterCalled {
		t.Error("OnAfterDownload hook not called")
	}
	if downloadedMedia == nil {
		t.Error("media not passed to hook")
	}
	if wasCached {
		t.Error("first download should not be cached")
	}
}

func TestMediaPipeline_HookReject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("should-not-reach"))
	}))
	defer server.Close()

	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	hook := &testHook{
		beforeFn: func(ctx context.Context, url string, req *MediaDownloadRequest) error {
			return fmt.Errorf("rejected by policy")
		},
	}
	pipeline.RegisterHook(hook)

	_, err := pipeline.Download(context.Background(), server.URL)
	if err == nil {
		t.Error("expected hook rejection error")
	}
}

func TestMediaPipeline_ErrorHook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.RetryCount = 0
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	var errorCount int32

	hook := &testHook{
		errorFn: func(ctx context.Context, url string, err error, attempt int) {
			atomic.AddInt32(&errorCount, 1)
		},
	}
	pipeline.RegisterHook(hook)

	_, _ = pipeline.Download(context.Background(), server.URL)

	if atomic.LoadInt32(&errorCount) == 0 {
		t.Error("OnDownloadError hook not called")
	}
}

func TestMediaPipeline_BatchCompleteHook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("batch"))
	}))
	defer server.Close()

	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	var batchCompleteCalled bool
	var batchResults []*MediaDownloadResult

	hook := &testHook{
		batchFn: func(ctx context.Context, results []*MediaDownloadResult) {
			batchCompleteCalled = true
			batchResults = results
		},
	}
	pipeline.RegisterHook(hook)

	urls := []string{server.URL + "/a", server.URL + "/b"}
	pipeline.DownloadBatch(context.Background(), urls)

	if !batchCompleteCalled {
		t.Error("OnBatchComplete hook not called")
	}
	if len(batchResults) != 2 {
		t.Errorf("expected 2 batch results, got %d", len(batchResults))
	}
}

func TestMediaPipeline_DownloadAndSave(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("save-test-data"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	cfg := DefaultMediaPipelineConfig()
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	path, err := pipeline.DownloadAndSave(context.Background(), server.URL, tmpDir)
	if err != nil {
		t.Fatalf("download and save failed: %v", err)
	}

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	if string(data) != "save-test-data" {
		t.Errorf("expected 'save-test-data', got %s", string(data))
	}
}

func TestMediaPipeline_Stats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("stats-test"))
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	_, _ = pipeline.Download(context.Background(), server.URL)

	stats := pipeline.Stats()

	if stats.DownloadsTotal != 1 {
		t.Errorf("expected 1 total download, got %d", stats.DownloadsTotal)
	}
	if stats.BytesDownloaded == 0 {
		t.Error("expected non-zero bytes downloaded")
	}
	if stats.LastDownloadTime.IsZero() {
		t.Error("expected non-zero last download time")
	}
}

func TestMediaPipeline_CacheStats(t *testing.T) {
	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	stats := pipeline.CacheStats()

	if stats.ItemCount < 0 {
		t.Error("expected non-negative item count")
	}
}

func TestMediaPipeline_ClearCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("clear-test"))
	}))
	defer server.Close()

	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	_, _ = pipeline.Download(context.Background(), server.URL)

	if pipeline.Cache().Len() == 0 {
		t.Fatal("expected cache to have items")
	}

	pipeline.ClearCache()

	if pipeline.Cache().Len() != 0 {
		t.Errorf("expected 0 items after clear, got %d", pipeline.Cache().Len())
	}
}

func TestMediaPipeline_CleanupCache(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	cfg.CacheConfig = MediaCacheConfig{
		MaxItems: 100,
		MaxBytes: 10 * 1024 * 1024,
		TTL:      50 * time.Millisecond,
	}

	pipeline := NewMediaPipeline(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("cleanup-test"))
	}))
	defer server.Close()

	_, _ = pipeline.Download(context.Background(), server.URL)

	time.Sleep(100 * time.Millisecond)

	removed := pipeline.CleanupCache()

	if removed != 1 {
		t.Errorf("expected 1 expired item cleaned up, got %d", removed)
	}
}

func TestMediaPipeline_EnableDisableCache(t *testing.T) {
	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	if pipeline.Cache() == nil {
		t.Fatal("expected cache to be enabled by default")
	}

	pipeline.DisableCache()

	if pipeline.Cache() != nil {
		t.Error("expected cache to be nil after disable")
	}

	pipeline.EnableCache(DefaultMediaCacheConfig())

	if pipeline.Cache() == nil {
		t.Error("expected cache to be non-nil after enable")
	}
}

func TestMediaPipeline_SetCache(t *testing.T) {
	pipeline := NewMediaPipeline(DefaultMediaPipelineConfig())

	customCache := NewMediaCache(MediaCacheConfig{
		MaxItems: 10,
		MaxBytes: 1024,
		TTL:      1 * time.Minute,
	})

	pipeline.SetCache(customCache)

	if pipeline.Cache() != customCache {
		t.Error("expected custom cache to be set")
	}
}

func TestMediaPipeline_ConcurrentDownloads(t *testing.T) {
	var activeRequests int32
	var maxActive int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&activeRequests, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&activeRequests, -1)

		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("concurrent"))
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.MaxConcurrent = 5
	cfg.UseCache = false

	pipeline := NewMediaPipeline(cfg)

	urls := make([]string, 20)
	for i := 0; i < 20; i++ {
		urls[i] = fmt.Sprintf("%s/%d", server.URL, i)
	}

	results := pipeline.DownloadBatch(context.Background(), urls)

	successCount := 0
	for _, r := range results {
		if r.Error == nil {
			successCount++
		}
	}

	if successCount != 20 {
		t.Errorf("expected 20 successful downloads, got %d", successCount)
	}

	if atomic.LoadInt32(&maxActive) > 5 {
		t.Errorf("expected max %d concurrent requests, got %d", 5, maxActive)
	}
}

func TestMediaPipeline_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Write([]byte("too-late"))
	}))
	defer server.Close()

	cfg := DefaultMediaPipelineConfig()
	cfg.UseCache = false
	cfg.RetryCount = 0

	pipeline := NewMediaPipeline(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pipeline.Download(ctx, server.URL)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

type testHook struct {
	beforeFn func(ctx context.Context, url string, req *MediaDownloadRequest) error
	afterFn  func(ctx context.Context, media *Media, cached bool) error
	errorFn  func(ctx context.Context, url string, err error, attempt int)
	batchFn  func(ctx context.Context, results []*MediaDownloadResult)
}

func (h *testHook) OnBeforeDownload(ctx context.Context, url string, req *MediaDownloadRequest) error {
	if h.beforeFn != nil {
		return h.beforeFn(ctx, url, req)
	}
	return nil
}

func (h *testHook) OnAfterDownload(ctx context.Context, media *Media, cached bool) error {
	if h.afterFn != nil {
		return h.afterFn(ctx, media, cached)
	}
	return nil
}

func (h *testHook) OnDownloadError(ctx context.Context, url string, err error, attempt int) {
	if h.errorFn != nil {
		h.errorFn(ctx, url, err, attempt)
	}
}

func (h *testHook) OnBatchComplete(ctx context.Context, results []*MediaDownloadResult) {
	if h.batchFn != nil {
		h.batchFn(ctx, results)
	}
}
