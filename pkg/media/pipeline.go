package media

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type MediaPipeline struct {
	mu         sync.RWMutex
	config     MediaPipelineConfig
	cache      *MediaCache
	processor  *Processor
	hooks      []MediaPipelineHook
	httpClient *http.Client
	inflight   sync.Map
	stats      MediaPipelineStats
}

type MediaPipelineConfig struct {
	Enabled          bool
	MaxDownloadSize  int64
	MaxConcurrent    int
	Timeout          time.Duration
	RetryCount       int
	RetryDelay       time.Duration
	UseCache         bool
	CacheConfig      MediaCacheConfig
	UserAgent        string
	AllowedMimeTypes []string
	BlockedSchemes   []string
	Transcode        bool
	ImageOptions     ImageOptions
	AudioOptions     AudioOptions
	VideoOptions     VideoOptions
	Storage          StorageBackend
	AutoSave         bool
	Signer           *URLSigner
	AutoSign         bool
	SignExpiry       time.Duration
}

func DefaultMediaPipelineConfig() MediaPipelineConfig {
	return MediaPipelineConfig{
		Enabled:         true,
		MaxDownloadSize: 100 * 1024 * 1024,
		MaxConcurrent:   10,
		Timeout:         60 * time.Second,
		RetryCount:      3,
		RetryDelay:      1 * time.Second,
		UseCache:        true,
		UserAgent:       "AnyClaw-MediaPipeline/1.0",
		AllowedMimeTypes: []string{
			"image/jpeg", "image/png", "image/gif", "image/webp", "image/svg+xml",
			"audio/mpeg", "audio/wav", "audio/ogg", "audio/mp4", "audio/webm",
			"video/mp4", "video/webm", "video/ogg", "video/quicktime",
			"application/pdf", "application/msword",
			"application/octet-stream",
		},
		BlockedSchemes: []string{"file", "data"},
	}
}

type MediaPipelineHook interface {
	OnBeforeDownload(ctx context.Context, url string, req *MediaDownloadRequest) error
	OnAfterDownload(ctx context.Context, media *Media, cached bool) error
	OnDownloadError(ctx context.Context, url string, err error, attempt int)
	OnBatchComplete(ctx context.Context, results []*MediaDownloadResult)
}

type MediaDownloadRequest struct {
	URL         string
	MaxSize     int64
	AcceptTypes []string
	Headers     map[string]string
	Metadata    map[string]any
}

type MediaDownloadResult struct {
	Media  *Media
	URL    string
	Cached bool
	Error  error
}

type MediaPipelineStats struct {
	DownloadsTotal   int
	DownloadsCached  int
	DownloadsFailed  int
	BytesDownloaded  int64
	AverageLatency   time.Duration
	ActiveDownloads  int
	LastDownloadTime time.Time
}

func NewMediaPipeline(cfg MediaPipelineConfig) *MediaPipeline {
	if cfg.MaxDownloadSize <= 0 {
		cfg.MaxDownloadSize = 100 * 1024 * 1024
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 10
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.RetryCount <= 0 {
		cfg.RetryCount = 3
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 1 * time.Second
	}

	p := &MediaPipeline{
		config:    cfg,
		processor: NewProcessor(""),
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}

	if p.config.UseCache {
		p.cache = NewMediaCache(p.config.CacheConfig)
	}

	return p
}

func (p *MediaPipeline) RegisterHook(hook MediaPipelineHook) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, hook)
}

func (p *MediaPipeline) SetCache(cache *MediaCache) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = cache
}

func (p *MediaPipeline) Cache() *MediaCache {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cache
}

func (p *MediaPipeline) EnableCache(cfg MediaCacheConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = NewMediaCache(cfg)
	p.config.UseCache = true
}

func (p *MediaPipeline) DisableCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = nil
	p.config.UseCache = false
}

func (p *MediaPipeline) Download(ctx context.Context, url string) (*Media, error) {
	return p.DownloadWithOptions(ctx, &MediaDownloadRequest{URL: url})
}

func (p *MediaPipeline) DownloadWithOptions(ctx context.Context, req *MediaDownloadRequest) (*Media, error) {
	p.mu.RLock()
	hooks := make([]MediaPipelineHook, len(p.hooks))
	copy(hooks, p.hooks)
	cache := p.cache
	maxSize := p.config.MaxDownloadSize
	retryCount := p.config.RetryCount
	retryDelay := p.config.RetryDelay
	userAgent := p.config.UserAgent
	allowedTypes := p.config.AllowedMimeTypes
	blockedSchemes := p.config.BlockedSchemes
	p.mu.RUnlock()

	if req.URL == "" {
		return nil, fmt.Errorf("media-pipeline: empty URL")
	}

	if !isAllowedScheme(req.URL, blockedSchemes) {
		return nil, fmt.Errorf("media-pipeline: blocked URL scheme: %s", req.URL)
	}

	maxReqSize := req.MaxSize
	if maxReqSize <= 0 {
		maxReqSize = maxSize
	}

	for _, hook := range hooks {
		if err := hook.OnBeforeDownload(ctx, req.URL, req); err != nil {
			return nil, fmt.Errorf("media-pipeline: hook rejected download: %w", err)
		}
	}

	cacheKey := MakeMediaCacheKey(req.URL)

	if cache != nil {
		if media, ok := cache.Get(cacheKey); ok {
			for _, hook := range hooks {
				_ = hook.OnAfterDownload(ctx, media, true)
			}

			p.mu.Lock()
			p.stats.DownloadsCached++
			p.stats.DownloadsTotal++
			p.stats.LastDownloadTime = time.Now()
			p.mu.Unlock()

			return media, nil
		}
	}

	_, inflight := p.inflight.LoadOrStore(req.URL, make(chan struct{}))
	if inflight {
		p.mu.Lock()
		p.stats.ActiveDownloads++
		p.mu.Unlock()

		defer func() {
			p.mu.Lock()
			p.stats.ActiveDownloads--
			p.mu.Unlock()
		}()
	} else {
		defer p.inflight.Delete(req.URL)
	}

	var media *Media
	var lastErr error

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay * time.Duration(attempt)):
			}
		}

		media, lastErr = p.doDownload(ctx, req.URL, maxReqSize, userAgent, allowedTypes)
		if lastErr == nil {
			break
		}

		for _, hook := range hooks {
			hook.OnDownloadError(ctx, req.URL, lastErr, attempt+1)
		}
	}

	if lastErr != nil {
		p.mu.Lock()
		p.stats.DownloadsFailed++
		p.stats.DownloadsTotal++
		p.mu.Unlock()
		return nil, fmt.Errorf("media-pipeline: download failed after %d attempts: %w", retryCount+1, lastErr)
	}

	if cache != nil {
		cache.Set(cacheKey, media)
	}

	for _, hook := range hooks {
		_ = hook.OnAfterDownload(ctx, media, false)
	}

	p.mu.Lock()
	p.stats.DownloadsTotal++
	p.stats.BytesDownloaded += media.Size
	p.stats.LastDownloadTime = time.Now()
	p.mu.Unlock()

	return media, nil
}

func (p *MediaPipeline) DownloadBatch(ctx context.Context, urls []string) []*MediaDownloadResult {
	results := make([]*MediaDownloadResult, len(urls))
	sem := make(chan struct{}, p.config.MaxConcurrent)
	var wg sync.WaitGroup

	startTime := time.Now()

	for i, url := range urls {
		sem <- struct{}{}
		wg.Add(1)

		go func(idx int, u string) {
			defer wg.Done()
			defer func() { <-sem }()

			result := &MediaDownloadResult{URL: u}

			media, err := p.Download(ctx, u)
			if err != nil {
				result.Error = err
			} else {
				result.Media = media
			}

			results[idx] = result
		}(i, url)
	}

	wg.Wait()

	elapsed := time.Since(startTime)

	p.mu.RLock()
	hooks := make([]MediaPipelineHook, len(p.hooks))
	copy(hooks, p.hooks)
	p.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnBatchComplete(ctx, results)
	}

	p.mu.Lock()
	if len(urls) > 0 {
		p.stats.AverageLatency = elapsed / time.Duration(len(urls))
	}
	p.mu.Unlock()

	return results
}

func (p *MediaPipeline) DownloadAndSave(ctx context.Context, url, destDir string) (string, error) {
	media, err := p.Download(ctx, url)
	if err != nil {
		return "", err
	}

	if destDir == "" {
		destDir = p.processor.storagePath
	}

	if destDir == "" {
		return "", fmt.Errorf("media-pipeline: no destination directory configured")
	}

	ext := extensionFromMime(media.MimeType)
	filename := fmt.Sprintf("%s%s", media.ID, ext)
	path := filepath.Join(destDir, filename)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("media-pipeline: failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, media.Data, 0644); err != nil {
		return "", fmt.Errorf("media-pipeline: failed to save file: %w", err)
	}

	media.Path = path

	return path, nil
}

func (p *MediaPipeline) Stats() MediaPipelineStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

func (p *MediaPipeline) CacheStats() MediaCacheStats {
	p.mu.RLock()
	cache := p.cache
	p.mu.RUnlock()

	if cache == nil {
		return MediaCacheStats{}
	}

	return cache.Stats()
}

func (p *MediaPipeline) ClearCache() {
	p.mu.RLock()
	cache := p.cache
	p.mu.RUnlock()

	if cache != nil {
		cache.Clear()
	}
}

func (p *MediaPipeline) CleanupCache() int {
	p.mu.RLock()
	cache := p.cache
	p.mu.RUnlock()

	if cache == nil {
		return 0
	}

	return cache.Cleanup()
}

func (p *MediaPipeline) SaveToStorage(ctx context.Context, media *Media) (string, error) {
	storage := p.config.Storage
	if storage == nil {
		return "", fmt.Errorf("media-pipeline: no storage backend configured")
	}

	ext := extensionFromMime(media.MimeType)
	key := fmt.Sprintf("%s%s", media.ID, ext)

	opts := StoragePutOptions{
		MimeType: media.MimeType,
	}

	obj, err := storage.Put(ctx, key, media.Data, opts)
	if err != nil {
		return "", fmt.Errorf("media-pipeline: save to storage: %w", err)
	}

	media.Path = obj.URL
	media.URL = obj.URL

	return obj.URL, nil
}

func (p *MediaPipeline) LoadFromStorage(ctx context.Context, key string) (*Media, error) {
	storage := p.config.Storage
	if storage == nil {
		return nil, fmt.Errorf("media-pipeline: no storage backend configured")
	}

	obj, err := storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("media-pipeline: load from storage: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(obj.Metadata["data"])
	if err != nil {
		return nil, fmt.Errorf("media-pipeline: decode stored data: %w", err)
	}

	return &Media{
		ID:       generateID(),
		Type:     p.processor.guessType(obj.MimeType),
		Name:     filepath.Base(key),
		Size:     obj.Size,
		MimeType: obj.MimeType,
		Data:     data,
		Base64:   base64.StdEncoding.EncodeToString(data),
		Path:     obj.URL,
		URL:      obj.URL,
	}, nil
}

func (p *MediaPipeline) DeleteFromStorage(ctx context.Context, key string) error {
	storage := p.config.Storage
	if storage == nil {
		return fmt.Errorf("media-pipeline: no storage backend configured")
	}
	return storage.Delete(ctx, key)
}

func (p *MediaPipeline) StorageURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	storage := p.config.Storage
	if storage == nil {
		return "", fmt.Errorf("media-pipeline: no storage backend configured")
	}
	return storage.URL(ctx, key, expires)
}

func (p *MediaPipeline) SignURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	signer := p.config.Signer
	if signer == nil {
		return "", fmt.Errorf("media-pipeline: no URL signer configured")
	}

	storage := p.config.Storage
	if storage != nil {
		return signer.SignStorageObject(storage, key, expires)
	}

	baseURL := key
	return signer.SignURL(baseURL, key, expires, nil)
}

func (p *MediaPipeline) VerifySignedURL(ctx context.Context, signedURL string) (*SignedURLResult, error) {
	signer := p.config.Signer
	if signer == nil {
		return nil, fmt.Errorf("media-pipeline: no URL signer configured")
	}

	baseURL, expireTime, metadata, err := signer.VerifyURL(signedURL)
	if err != nil {
		return nil, err
	}

	remaining := expireTime.Sub(time.Now().UTC())

	return &SignedURLResult{
		URL:       baseURL,
		ExpiresAt: expireTime,
		Remaining: remaining,
		Metadata:  metadata,
		IsRevoked: remaining < 0,
	}, nil
}

func (p *MediaPipeline) RevokeSignedURL(ctx context.Context, signedURL string) error {
	signer := p.config.Signer
	if signer == nil {
		return fmt.Errorf("media-pipeline: no URL signer configured")
	}
	return signer.RevokeURL(signedURL)
}

func (p *MediaPipeline) SignMediaURL(ctx context.Context, media *Media, expires time.Duration) (string, error) {
	signer := p.config.Signer
	if signer == nil {
		return "", fmt.Errorf("media-pipeline: no URL signer configured")
	}

	storage := p.config.Storage
	if storage != nil && media.Metadata["storage-key"] != "" {
		key := media.Metadata["storage-key"].(string)
		return signer.SignStorageObject(storage, key, expires)
	}

	key := media.ID
	if media.Name != "" {
		key = media.Name
	}

	metadata := map[string]string{
		"m_type": string(media.Type),
		"m_mime": media.MimeType,
	}

	baseURL := media.URL
	if baseURL == "" {
		baseURL = "https://media.example.com/" + key
	}

	return signer.SignURL(baseURL, key, expires, metadata)
}

func (p *MediaPipeline) CleanupRevokedURLs() int {
	signer := p.config.Signer
	if signer == nil {
		return 0
	}
	return signer.CleanupRevoked()
}

func (p *MediaPipeline) doDownload(ctx context.Context, url string, maxSize int64, userAgent string, allowedTypes []string) (*Media, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download failed: HTTP %d %s", resp.StatusCode, resp.Status)
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, fmt.Errorf("redirect without Location header: HTTP %d", resp.StatusCode)
		}
		return p.doDownload(ctx, loc, maxSize, userAgent, allowedTypes)
	}

	contentLength := resp.ContentLength
	if contentLength > maxSize {
		return nil, fmt.Errorf("content too large: %d bytes (max %d)", contentLength, maxSize)
	}

	mediaType := resp.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	if len(allowedTypes) > 0 {
		allowed := false
		for _, t := range allowedTypes {
			if t == mediaType || t == "application/octet-stream" {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("unsupported MIME type: %s", mediaType)
		}
	}

	reader := io.LimitReader(resp.Body, maxSize)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty response body")
	}

	detected := DetectMediaType(data, filenameFromURL(url), mediaType)

	media := &Media{
		ID:       generateID(),
		Type:     detected.Type,
		MimeType: detected.MimeType,
		Size:     int64(len(data)),
		Data:     data,
		URL:      url,
		Metadata: map[string]any{
			"status_code":    resp.StatusCode,
			"content_length": contentLength,
			"downloaded_at":  time.Now().Unix(),
			"format":         string(detected.Format),
		},
	}

	_, _ = p.processor.Process(ctx, media)

	if p.config.Transcode {
		if err := p.transcodeMedia(ctx, media); err != nil {
			return nil, fmt.Errorf("media-pipeline: transcode failed: %w", err)
		}
	}

	if p.config.AutoSave && p.config.Storage != nil {
		storage := p.config.Storage
		ext := extensionFromMime(media.MimeType)
		key := fmt.Sprintf("%s%s", media.ID, ext)

		opts := StoragePutOptions{
			MimeType: media.MimeType,
			Metadata: map[string]string{
				"media-type": string(media.Type),
				"source-url": url,
			},
		}

		obj, err := storage.Put(ctx, key, media.Data, opts)
		if err != nil {
			return nil, fmt.Errorf("media-pipeline: auto-save failed: %w", err)
		}

		media.Path = obj.URL
		media.URL = obj.URL
		media.Metadata["storage-key"] = key
		media.Metadata["storage-url"] = obj.URL

		if p.config.AutoSign && p.config.Signer != nil {
			signExpires := p.config.SignExpiry
			if signExpires <= 0 {
				signExpires = 24 * time.Hour
			}

			signedURL, err := p.config.Signer.SignStorageObject(storage, key, signExpires)
			if err != nil {
				return nil, fmt.Errorf("media-pipeline: auto-sign failed: %w", err)
			}

			media.Metadata["signed-url"] = signedURL
			media.Metadata["signed-expires"] = time.Now().UTC().Add(signExpires).Format(time.RFC3339)
		}
	}

	return media, nil
}

func (p *MediaPipeline) transcodeMedia(ctx context.Context, media *Media) error {
	transcoder := p.processor.Transcoder()
	if transcoder == nil {
		return nil
	}

	switch media.Type {
	case TypeImage:
		opts := p.config.ImageOptions
		if opts.Format == FormatUnknown {
			opts.Format = FormatJPEG
		}
		if opts.Quality == 0 {
			opts.Quality = int(QualityHigh)
		}
		result, err := transcoder.CompressImage(media.Data, opts)
		if err != nil {
			return fmt.Errorf("compress image: %w", err)
		}
		media.Data = result
		media.Size = int64(len(result))
		media.Base64 = ""
		detected := DetectMediaType(result, media.Name, media.MimeType)
		if detected.Format != FormatUnknown {
			media.MimeType = detected.MimeType
			if media.Metadata == nil {
				media.Metadata = make(map[string]any)
			}
			media.Metadata["format"] = string(detected.Format)
			media.Metadata["compressed"] = true
		}
	case TypeAudio:
		opts := p.config.AudioOptions
		if opts.Format == FormatUnknown {
			opts.Format = FormatMP3
		}
		result, err := transcoder.TranscodeAudio(ctx, media.Data, opts)
		if err != nil {
			return fmt.Errorf("transcode audio: %w", err)
		}
		media.Data = result
		media.Size = int64(len(result))
		media.Base64 = ""
		media.MimeType = formatToMIME(opts.Format)
		if media.Metadata == nil {
			media.Metadata = make(map[string]any)
		}
		media.Metadata["format"] = string(opts.Format)
		media.Metadata["compressed"] = true
	case TypeVideo:
		opts := p.config.VideoOptions
		if opts.Format == FormatUnknown {
			opts.Format = FormatMP4
		}
		result, err := transcoder.TranscodeVideo(ctx, media.Data, opts)
		if err != nil {
			return fmt.Errorf("transcode video: %w", err)
		}
		media.Data = result
		media.Size = int64(len(result))
		media.Base64 = ""
		media.MimeType = formatToMIME(opts.Format)
		if media.Metadata == nil {
			media.Metadata = make(map[string]any)
		}
		media.Metadata["format"] = string(opts.Format)
		media.Metadata["compressed"] = true
	}

	return nil
}

func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return filepath.Base(u.Path)
}

func isAllowedScheme(url string, blocked []string) bool {
	for _, scheme := range blocked {
		if len(url) > len(scheme)+1 && url[:len(scheme)] == scheme && url[len(scheme)] == ':' {
			return false
		}
	}
	return true
}
