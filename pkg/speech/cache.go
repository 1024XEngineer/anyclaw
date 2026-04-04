package speech

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type AudioCache struct {
	mu           sync.RWMutex
	items        map[string]audioCacheItem
	maxSize      int
	maxBytes     int64
	currentBytes int64
	ttl          time.Duration
}

type audioCacheItem struct {
	result    *AudioResult
	expiresAt time.Time
	sizeBytes int64
}

type CacheConfig struct {
	MaxSize  int
	MaxBytes int64
	TTL      time.Duration
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxSize:  1000,
		MaxBytes: 500 * 1024 * 1024,
		TTL:      24 * time.Hour,
	}
}

func NewAudioCache(cfg CacheConfig) *AudioCache {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 1000
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 500 * 1024 * 1024
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}

	return &AudioCache{
		items:    make(map[string]audioCacheItem),
		maxSize:  cfg.MaxSize,
		maxBytes: cfg.MaxBytes,
		ttl:      cfg.TTL,
	}
}

func (c *AudioCache) Get(key string) (*AudioResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(item.expiresAt) {
		return nil, false
	}

	return item.result, true
}

func (c *AudioCache) Set(key string, result *AudioResult) {
	if result == nil || len(result.Data) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	sizeBytes := int64(len(result.Data))

	if sizeBytes > c.maxBytes {
		return
	}

	if len(c.items) >= c.maxSize || c.currentBytes+sizeBytes > c.maxBytes {
		c.evict(sizeBytes)
	}

	c.items[key] = audioCacheItem{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
		sizeBytes: sizeBytes,
	}
	c.currentBytes += sizeBytes
}

func (c *AudioCache) evict(neededBytes int64) {
	now := time.Now()

	for k, v := range c.items {
		if now.After(v.expiresAt) {
			c.currentBytes -= v.sizeBytes
			delete(c.items, k)
		}
	}

	if len(c.items) >= c.maxSize || c.currentBytes+neededBytes > c.maxBytes {
		oldestKey := ""
		oldestTime := time.Now().Add(c.ttl)

		for k, v := range c.items {
			if v.expiresAt.Before(oldestTime) {
				oldestTime = v.expiresAt
				oldestKey = k
			}
		}

		if oldestKey != "" {
			item := c.items[oldestKey]
			c.currentBytes -= item.sizeBytes
			delete(c.items, oldestKey)
		}
	}

	if len(c.items) >= c.maxSize {
		count := 0
		half := c.maxSize / 2
		for k, v := range c.items {
			c.currentBytes -= v.sizeBytes
			delete(c.items, k)
			count++
			if count >= half {
				break
			}
		}
	}
}

func (c *AudioCache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		c.currentBytes -= item.sizeBytes
		delete(c.items, key)
	}
}

func (c *AudioCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *AudioCache) SizeBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentBytes
}

func (c *AudioCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]audioCacheItem)
	c.currentBytes = 0
}

func (c *AudioCache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0

	for k, v := range c.items {
		if now.After(v.expiresAt) {
			c.currentBytes -= v.sizeBytes
			delete(c.items, k)
			count++
		}
	}

	return count
}

func MakeCacheKey(text string, provider string, opts ...SynthesizeOption) string {
	options := SynthesizeOptions{
		Voice:  "default",
		Speed:  1.0,
		Format: FormatMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	raw := fmt.Sprintf("%s|%s|%s|%.2f|%s|%s",
		provider,
		options.Voice,
		text,
		options.Speed,
		options.Format,
		options.Language,
	)

	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
