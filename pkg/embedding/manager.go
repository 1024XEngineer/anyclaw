package embedding

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Manager struct {
	mu        sync.RWMutex
	providers []Provider
	current   int
	cache     *Cache
}

type Cache struct {
	mu      sync.RWMutex
	items   map[string]cacheItem
	maxSize int
	ttl     time.Duration
}

type cacheItem struct {
	value     []float32
	expiresAt time.Time
}

func NewCache(maxSize int, ttl time.Duration) *Cache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Cache{
		items:   make(map[string]cacheItem),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *Cache) Get(key string) ([]float32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

func (c *Cache) Set(key string, value []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= c.maxSize {
		c.evict()
	}

	c.items[key] = cacheItem{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) evict() {
	now := time.Now()
	for k, v := range c.items {
		if now.After(v.expiresAt) {
			delete(c.items, k)
		}
	}

	if len(c.items) >= c.maxSize {
		count := 0
		half := c.maxSize / 2
		for k := range c.items {
			delete(c.items, k)
			count++
			if count >= half {
				break
			}
		}
	}
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]cacheItem)
}

func NewManager(providers []Provider, cache *Cache) (*Manager, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("embedding: at least one provider is required")
	}
	if cache == nil {
		cache = NewCache(10000, 24*time.Hour)
	}
	return &Manager{
		providers: providers,
		cache:     cache,
	}, nil
}

func (m *Manager) Embed(ctx context.Context, text string) ([]float32, error) {
	if cached, ok := m.cache.Get(text); ok {
		return cached, nil
	}

	var lastErr error
	for i, provider := range m.providers {
		embedding, err := provider.Embed(ctx, text)
		if err == nil {
			m.mu.Lock()
			m.current = i
			m.mu.Unlock()
			m.cache.Set(text, embedding)
			return embedding, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("embedding: all providers failed, last error: %w", lastErr)
}

func (m *Manager) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, text := range texts {
		embedding, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, embedding)
	}
	return results, nil
}

func (m *Manager) CurrentProvider() Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.providers) == 0 {
		return nil
	}
	return m.providers[m.current]
}

func (m *Manager) Providers() []Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers
}

func (m *Manager) Cache() *Cache {
	return m.cache
}
