package providers

import "sync"

// Registry stores provider implementations by ID.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register stores a provider by its ID.
func (r *Registry) Register(provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.ID()] = provider
}

// Get looks up a provider by ID.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[id]
	return provider, ok
}

// List returns all registered providers.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		out = append(out, provider)
	}
	return out
}
