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
}

func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]Provider),
	}
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
