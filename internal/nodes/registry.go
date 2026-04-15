package nodes

import "sync"

// Registry stores online node capability hosts.
type Registry struct {
	mu    sync.RWMutex
	nodes map[string]Node
}

// NewRegistry creates an empty node registry.
func NewRegistry() *Registry {
	return &Registry{nodes: make(map[string]Node)}
}

// Register stores a node by ID.
func (r *Registry) Register(node Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[node.ID()] = node
}

// Get looks up a node by ID.
func (r *Registry) Get(id string) (Node, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	node, ok := r.nodes[id]
	return node, ok
}
