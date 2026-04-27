package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type MediaCacheConfig struct {
	MaxItems     int
	MaxBytes     int64
	TTL          time.Duration
	DiskPath     string
	PersistIndex bool
}

func DefaultMediaCacheConfig() MediaCacheConfig {
	return MediaCacheConfig{
		MaxItems:     5000,
		MaxBytes:     1024 * 1024 * 1024,
		TTL:          7 * 24 * time.Hour,
		PersistIndex: true,
	}
}

type MediaCacheStats struct {
	Hits        int
	Misses      int
	Evictions   int
	Expired     int
	TotalSize   int64
	ItemCount   int
	DiskItems   int
	LastHitRate float64
}

type MediaCache struct {
	mu            sync.RWMutex
	items         map[string]*mediaCacheItem
	maxItems      int
	maxBytes      int64
	currentBytes  int64
	ttl           time.Duration
	diskPath      string
	persistIndex  bool
	stats         MediaCacheStats
	totalRequests int
}

type mediaCacheItem struct {
	Media       *Media
	ExpiresAt   time.Time
	CreatedAt   time.Time
	SizeBytes   int64
	AccessCount int
	LastAccess  time.Time
}

type mediaCacheIndex struct {
	Items map[string]mediaCacheIndexEntry `json:"items"`
}

type mediaCacheIndexEntry struct {
	Key       string    `json:"key"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func NewMediaCache(cfg MediaCacheConfig) *MediaCache {
	if cfg.MaxItems <= 0 {
		cfg.MaxItems = 5000
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 1024 * 1024 * 1024
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 7 * 24 * time.Hour
	}

	mc := &MediaCache{
		items:        make(map[string]*mediaCacheItem),
		maxItems:     cfg.MaxItems,
		maxBytes:     cfg.MaxBytes,
		ttl:          cfg.TTL,
		diskPath:     cfg.DiskPath,
		persistIndex: cfg.PersistIndex,
	}

	if mc.diskPath != "" {
		_ = os.MkdirAll(mc.diskPath, 0755)
		if mc.persistIndex {
			mc.loadIndex()
		}
	}

	return mc
}

func (c *MediaCache) Get(key string) (*Media, bool) {
	c.mu.Lock()

	item, ok := c.items[key]
	if !ok {
		c.stats.Misses++
		c.totalRequests++
		c.mu.Unlock()
		return nil, false
	}

	if time.Now().After(item.ExpiresAt) {
		c.currentBytes -= item.SizeBytes
		delete(c.items, key)
		c.stats.Expired++
		c.stats.Misses++
		c.totalRequests++
		c.mu.Unlock()
		return nil, false
	}

	item.AccessCount++
	item.LastAccess = time.Now()

	if item.Media != nil && len(item.Media.Data) > 0 {
		c.stats.Hits++
		c.totalRequests++
		c.updateHitRate()
		c.mu.Unlock()
		return item.Media, true
	}

	if c.diskPath != "" {
		diskData, err := c.loadFromDisk(key)
		if err == nil && len(diskData) > 0 {
			item.Media.Data = diskData
			c.stats.Hits++
			c.totalRequests++
			c.updateHitRate()
			c.mu.Unlock()
			return item.Media, true
		}
	}

	c.currentBytes -= item.SizeBytes
	delete(c.items, key)
	c.stats.Misses++
	c.totalRequests++
	c.mu.Unlock()
	return nil, false
}

func (c *MediaCache) Set(key string, media *Media) {
	if media == nil {
		return
	}

	sizeBytes := media.Size
	if sizeBytes == 0 {
		sizeBytes = int64(len(media.Data))
	}

	if sizeBytes > c.maxBytes {
		return
	}

	c.mu.Lock()

	if existing, ok := c.items[key]; ok {
		c.currentBytes -= existing.SizeBytes
		delete(c.items, key)
	}

	if len(c.items) >= c.maxItems || c.currentBytes+sizeBytes > c.maxBytes {
		c.evict(sizeBytes)
	}

	item := &mediaCacheItem{
		Media:       media,
		ExpiresAt:   time.Now().Add(c.ttl),
		CreatedAt:   time.Now(),
		SizeBytes:   sizeBytes,
		AccessCount: 1,
		LastAccess:  time.Now(),
	}

	c.items[key] = item
	c.currentBytes += sizeBytes

	if c.diskPath != "" {
		if len(media.Data) > 0 {
			_ = c.saveToDisk(key, media.Data)
		}
		if c.persistIndex {
			_ = c.saveIndex()
		}
	}

	c.mu.Unlock()
}

func (c *MediaCache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		c.currentBytes -= item.SizeBytes
		delete(c.items, key)
		if c.diskPath != "" {
			_ = c.deleteFromDisk(key)
		}
	}
}

func (c *MediaCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*mediaCacheItem)
	c.currentBytes = 0

	if c.diskPath != "" {
		_ = os.RemoveAll(c.diskPath)
		_ = os.MkdirAll(c.diskPath, 0755)
	}
}

func (c *MediaCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *MediaCache) SizeBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentBytes
}

func (c *MediaCache) Stats() MediaCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := c.stats
	stats.ItemCount = len(c.items)
	stats.TotalSize = c.currentBytes
	stats.DiskItems = c.diskItemCount()
	return stats
}

func (c *MediaCache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0

	for k, v := range c.items {
		if now.After(v.ExpiresAt) {
			c.currentBytes -= v.SizeBytes
			delete(c.items, k)
			if c.diskPath != "" {
				_ = c.deleteFromDisk(k)
			}
			count++
		}
	}

	return count
}

func (c *MediaCache) evict(neededBytes int64) {
	now := time.Now()

	for k, v := range c.items {
		if now.After(v.ExpiresAt) {
			c.currentBytes -= v.SizeBytes
			delete(c.items, k)
			c.stats.Evictions++
			if c.diskPath != "" {
				_ = c.deleteFromDisk(k)
			}
		}
	}

	if len(c.items) >= c.maxItems || c.currentBytes+neededBytes > c.maxBytes {
		oldestKey := ""
		oldestAccess := time.Now()

		for k, v := range c.items {
			if v.LastAccess.Before(oldestAccess) {
				oldestAccess = v.LastAccess
				oldestKey = k
			}
		}

		if oldestKey != "" {
			item := c.items[oldestKey]
			c.currentBytes -= item.SizeBytes
			delete(c.items, oldestKey)
			c.stats.Evictions++
			if c.diskPath != "" {
				_ = c.deleteFromDisk(oldestKey)
			}
		}
	}

	if len(c.items) >= c.maxItems {
		count := 0
		half := c.maxItems / 2
		for k, v := range c.items {
			c.currentBytes -= v.SizeBytes
			delete(c.items, k)
			c.stats.Evictions++
			if c.diskPath != "" {
				_ = c.deleteFromDisk(k)
			}
			count++
			if count >= half {
				break
			}
		}
	}
}

func (c *MediaCache) saveToDisk(key string, data []byte) error {
	if c.diskPath == "" {
		return nil
	}

	filename := keyToFilename(key)
	path := filepath.Join(c.diskPath, filename)

	return os.WriteFile(path, data, 0644)
}

func (c *MediaCache) loadFromDisk(key string) ([]byte, error) {
	if c.diskPath == "" {
		return nil, fmt.Errorf("no disk path configured")
	}

	filename := keyToFilename(key)
	path := filepath.Join(c.diskPath, filename)

	return os.ReadFile(path)
}

func (c *MediaCache) deleteFromDisk(key string) error {
	if c.diskPath == "" {
		return nil
	}

	filename := keyToFilename(key)
	path := filepath.Join(c.diskPath, filename)

	return os.Remove(path)
}

func (c *MediaCache) saveIndex() error {
	if c.diskPath == "" {
		return nil
	}

	index := mediaCacheIndex{
		Items: make(map[string]mediaCacheIndexEntry),
	}

	for k, v := range c.items {
		index.Items[k] = mediaCacheIndexEntry{
			Key:       k,
			URL:       v.Media.URL,
			Size:      v.SizeBytes,
			MimeType:  v.Media.MimeType,
			ExpiresAt: v.ExpiresAt,
			CreatedAt: v.CreatedAt,
		}
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	indexPath := filepath.Join(c.diskPath, "cache_index.json")
	return os.WriteFile(indexPath, data, 0644)
}

func (c *MediaCache) loadIndex() error {
	if c.diskPath == "" {
		return nil
	}

	indexPath := filepath.Join(c.diskPath, "cache_index.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index mediaCacheIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	now := time.Now()
	for k, entry := range index.Items {
		if now.After(entry.ExpiresAt) {
			continue
		}

		if c.currentBytes+entry.Size > c.maxBytes {
			break
		}

		if len(c.items) >= c.maxItems {
			break
		}

		media := &Media{
			ID:       k,
			URL:      entry.URL,
			Size:     entry.Size,
			MimeType: entry.MimeType,
		}

		c.items[k] = &mediaCacheItem{
			Media:       media,
			ExpiresAt:   entry.ExpiresAt,
			CreatedAt:   entry.CreatedAt,
			SizeBytes:   entry.Size,
			AccessCount: 0,
			LastAccess:  time.Time{},
		}
		c.currentBytes += entry.Size
	}

	return nil
}

func (c *MediaCache) diskItemCount() int {
	if c.diskPath == "" {
		return 0
	}

	entries, err := os.ReadDir(c.diskPath)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "cache_index.json" {
			count++
		}
	}
	return count
}

func (c *MediaCache) updateHitRate() {
	if c.totalRequests > 0 {
		c.stats.LastHitRate = float64(c.stats.Hits) / float64(c.totalRequests) * 100
	}
}

func MakeMediaCacheKey(url string, opts ...MediaCacheOption) string {
	options := mediaCacheOptions{
		MaxSize: 0,
		Format:  "",
	}
	for _, opt := range opts {
		opt(&options)
	}

	raw := fmt.Sprintf("%s|%d|%s", url, options.MaxSize, options.Format)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

type mediaCacheOptions struct {
	MaxSize int64
	Format  string
}

type MediaCacheOption func(*mediaCacheOptions)

func WithCacheMaxSize(size int64) MediaCacheOption {
	return func(o *mediaCacheOptions) {
		o.MaxSize = size
	}
}

func WithCacheFormat(format string) MediaCacheOption {
	return func(o *mediaCacheOptions) {
		o.Format = format
	}
}

func keyToFilename(key string) string {
	return key + ".cache"
}
