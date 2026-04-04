package speech

import (
	"context"
	"fmt"
	"sync"
)

type Manager struct {
	mu          sync.RWMutex
	providers   map[string]Provider
	defaultName string
	cache       *AudioCache
	cacheConfig CacheConfig
}

type ManagerOption func(*Manager)

func WithManagerCache(cfg CacheConfig) ManagerOption {
	return func(m *Manager) {
		m.cacheConfig = cfg
		m.cache = NewAudioCache(cfg)
	}
}

func WithManagerProvider(name string, provider Provider) ManagerOption {
	return func(m *Manager) {
		m.providers[name] = provider
		if m.defaultName == "" {
			m.defaultName = name
		}
	}
}

func WithManagerDefault(name string) ManagerOption {
	return func(m *Manager) {
		m.defaultName = name
	}
}

func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		providers:   make(map[string]Provider),
		cacheConfig: DefaultCacheConfig(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

func (m *Manager) Register(name string, provider Provider) error {
	if name == "" {
		return fmt.Errorf("tts: provider name cannot be empty")
	}
	if provider == nil {
		return fmt.Errorf("tts: provider cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[name]; exists {
		return fmt.Errorf("tts: provider already registered: %s", name)
	}

	m.providers[name] = provider

	if m.defaultName == "" {
		m.defaultName = name
	}

	return nil
}

func (m *Manager) Get(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("tts: provider not found: %s", name)
	}

	return provider, nil
}

func (m *Manager) GetDefault() (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.defaultName == "" {
		return nil, fmt.Errorf("tts: no default provider configured")
	}

	provider, ok := m.providers[m.defaultName]
	if !ok {
		return nil, fmt.Errorf("tts: default provider not found: %s", m.defaultName)
	}

	return provider, nil
}

func (m *Manager) SetDefault(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.providers[name]; !ok {
		return fmt.Errorf("tts: provider not found: %s", name)
	}

	m.defaultName = name
	return nil
}

func (m *Manager) Synthesize(ctx context.Context, text string, provider string, opts ...SynthesizeOption) (*AudioResult, error) {
	var p Provider
	var err error

	if provider != "" {
		p, err = m.Get(provider)
	} else {
		p, err = m.GetDefault()
	}

	if err != nil {
		return nil, err
	}

	if m.cache != nil {
		cacheKey := MakeCacheKey(text, provider, opts...)
		if cached, ok := m.cache.Get(cacheKey); ok {
			return cached, nil
		}

		result, err := p.Synthesize(ctx, text, opts...)
		if err == nil && result != nil {
			m.cache.Set(cacheKey, result)
		}
		return result, err
	}

	return p.Synthesize(ctx, text, opts...)
}

func (m *Manager) ListVoices(ctx context.Context, provider string) ([]Voice, error) {
	var p Provider
	var err error

	if provider != "" {
		p, err = m.Get(provider)
	} else {
		p, err = m.GetDefault()
	}

	if err != nil {
		return nil, err
	}

	return p.ListVoices(ctx)
}

func (m *Manager) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}

	return names
}

func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.providers[name]; !ok {
		return fmt.Errorf("tts: provider not found: %s", name)
	}

	delete(m.providers, name)

	if m.defaultName == name {
		m.defaultName = ""
		if len(m.providers) > 0 {
			for n := range m.providers {
				m.defaultName = n
				break
			}
		}
	}

	return nil
}

func (m *Manager) Cache() *AudioCache {
	return m.cache
}

func (m *Manager) EnableCache(cfg CacheConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheConfig = cfg
	m.cache = NewAudioCache(cfg)
}

func (m *Manager) DisableCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = nil
}

func (m *Manager) ClearCache() {
	if m.cache != nil {
		m.cache.Clear()
	}
}

func (m *Manager) CacheStats() (int, int64) {
	if m.cache == nil {
		return 0, 0
	}
	return m.cache.Len(), m.cache.SizeBytes()
}
