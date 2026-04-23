package input

import (
	"context"
	"sync"
	"time"
)

type InboundHandler func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error)

type StreamChunkHandler func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error)

type StreamAdapter interface {
	Adapter
	RunStream(ctx context.Context, handle StreamChunkHandler) error
}

type Status struct {
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	Running      bool      `json:"running"`
	Healthy      bool      `json:"healthy"`
	LastError    string    `json:"last_error,omitempty"`
	LastActivity time.Time `json:"last_activity,omitempty"`
}

type Adapter interface {
	Name() string
	Enabled() bool
	Run(ctx context.Context, handle InboundHandler) error
	Status() Status
}

type BaseAdapter struct {
	mu     sync.RWMutex
	status Status
}

func NewBaseAdapter(name string, enabled bool) BaseAdapter {
	return BaseAdapter{status: Status{Name: name, Enabled: enabled}}
}

func (b *BaseAdapter) Status() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *BaseAdapter) SetRunning(running bool) {
	b.mu.Lock()
	b.status.Running = running
	b.mu.Unlock()
}

func (b *BaseAdapter) SetError(err error) {
	b.mu.Lock()
	if err != nil {
		b.status.LastError = err.Error()
		b.status.Healthy = false
	} else {
		b.status.LastError = ""
		b.status.Healthy = true
	}
	b.mu.Unlock()
}

func (b *BaseAdapter) MarkActivity() {
	b.mu.Lock()
	b.status.LastActivity = time.Now().UTC()
	b.status.Healthy = true
	b.mu.Unlock()
}

type Manager struct {
	adapters []Adapter
}

func NewManager(adapters ...Adapter) *Manager {
	filtered := make([]Adapter, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			filtered = append(filtered, adapter)
		}
	}
	return &Manager{adapters: filtered}
}

func (m *Manager) Run(ctx context.Context, handle InboundHandler) {
	for _, adapter := range m.adapters {
		if !adapter.Enabled() {
			continue
		}
		go func(adapter Adapter) {
			_ = adapter.Run(ctx, handle)
		}(adapter)
	}
}

func (m *Manager) Statuses() []Status {
	statuses := make([]Status, 0, len(m.adapters))
	for _, adapter := range m.adapters {
		statuses = append(statuses, adapter.Status())
	}
	return statuses
}
