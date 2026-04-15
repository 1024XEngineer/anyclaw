package channels

import "sync"

// Registry stores channels by ID.
type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

// NewRegistry creates an empty channel registry.
func NewRegistry() *Registry {
	return &Registry{channels: make(map[string]Channel)}
}

// Register stores a channel by ID.
func (r *Registry) Register(channel Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[channel.ID()] = channel
}

// Get looks up a channel by ID.
func (r *Registry) Get(id string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	channel, ok := r.channels[id]
	return channel, ok
}

// List returns all registered channels.
func (r *Registry) List() []Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Channel, 0, len(r.channels))
	for _, channel := range r.channels {
		out = append(out, channel)
	}
	return out
}
