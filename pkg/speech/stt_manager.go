package speech

import (
	"context"
	"fmt"
	"sync"
)

type STTManager struct {
	mu          sync.RWMutex
	providers   map[string]STTProvider
	defaultName string
}

type STTManagerOption func(*STTManager)

func WithSTTManagerProvider(name string, provider STTProvider) STTManagerOption {
	return func(m *STTManager) {
		m.providers[name] = provider
		if m.defaultName == "" {
			m.defaultName = name
		}
	}
}

func WithSTTManagerDefault(name string) STTManagerOption {
	return func(m *STTManager) {
		m.defaultName = name
	}
}

func NewSTTManager(opts ...STTManagerOption) *STTManager {
	m := &STTManager{
		providers: make(map[string]STTProvider),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

func (m *STTManager) Register(name string, provider STTProvider) error {
	if name == "" {
		return fmt.Errorf("stt: provider name cannot be empty")
	}
	if provider == nil {
		return fmt.Errorf("stt: provider cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[name]; exists {
		return fmt.Errorf("stt: provider already registered: %s", name)
	}

	m.providers[name] = provider

	if m.defaultName == "" {
		m.defaultName = name
	}

	return nil
}

func (m *STTManager) Get(name string) (STTProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("stt: provider not found: %s", name)
	}

	return provider, nil
}

func (m *STTManager) GetDefault() (STTProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.defaultName == "" {
		return nil, fmt.Errorf("stt: no default provider configured")
	}

	provider, ok := m.providers[m.defaultName]
	if !ok {
		return nil, fmt.Errorf("stt: default provider not found: %s", m.defaultName)
	}

	return provider, nil
}

func (m *STTManager) SetDefault(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.providers[name]; !ok {
		return fmt.Errorf("stt: provider not found: %s", name)
	}

	m.defaultName = name
	return nil
}

func (m *STTManager) Transcribe(ctx context.Context, audio []byte, provider string, opts ...TranscribeOption) (*TranscriptResult, error) {
	var p STTProvider
	var err error

	if provider != "" {
		p, err = m.Get(provider)
	} else {
		p, err = m.GetDefault()
	}

	if err != nil {
		return nil, err
	}

	return p.Transcribe(ctx, audio, opts...)
}

func (m *STTManager) ListLanguages(ctx context.Context, provider string) ([]string, error) {
	var p STTProvider
	var err error

	if provider != "" {
		p, err = m.Get(provider)
	} else {
		p, err = m.GetDefault()
	}

	if err != nil {
		return nil, err
	}

	return p.ListLanguages(ctx)
}

func (m *STTManager) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}

	return names
}

func (m *STTManager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.providers[name]; !ok {
		return fmt.Errorf("stt: provider not found: %s", name)
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
