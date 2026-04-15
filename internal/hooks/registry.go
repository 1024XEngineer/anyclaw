package hooks

import (
	"sync"

	"anyclaw/pkg/sdk"
)

// Registry stores hooks by hook point.
type Registry struct {
	mu    sync.RWMutex
	hooks map[sdk.HookPoint][]sdk.Hook
}

// NewRegistry creates an empty hook registry.
func NewRegistry() *Registry {
	return &Registry{hooks: make(map[sdk.HookPoint][]sdk.Hook)}
}

// Register adds a hook to the registry.
func (r *Registry) Register(hook sdk.Hook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	point := hook.Point()
	r.hooks[point] = append(r.hooks[point], hook)
}

// List returns all hooks for a given hook point.
func (r *Registry) List(point sdk.HookPoint) []sdk.Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.hooks[point]
	out := make([]sdk.Hook, len(items))
	copy(out, items)
	return out
}
