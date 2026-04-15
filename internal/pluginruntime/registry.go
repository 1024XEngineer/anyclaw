package pluginruntime

import (
	"context"
	"sync"

	"anyclaw/pkg/sdk"
)

// Registry stores application plugins by ID.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]sdk.Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]sdk.Plugin)}
}

// Register stores a plugin by its ID.
func (r *Registry) Register(plugin sdk.Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[plugin.ID()] = plugin
}

// List returns all registered plugins.
func (r *Registry) List() []sdk.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]sdk.Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		out = append(out, plugin)
	}
	return out
}

// Boot registers all plugins into the provided app context.
func (r *Registry) Boot(ctx context.Context, app sdk.AppContext) error {
	for _, plugin := range r.List() {
		if err := plugin.Register(ctx, app); err != nil {
			return err
		}
	}
	return nil
}
