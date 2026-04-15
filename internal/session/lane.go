package session

import (
	"context"
	"sync"
)

// Lane serializes work for a single session.
type Lane interface {
	Submit(ctx context.Context, fn func(context.Context) error) error
}

// InMemoryLane is a mutex-backed lane for serialized session work.
type InMemoryLane struct {
	mu sync.Mutex
}

// Submit executes fn with exclusive access.
func (l *InMemoryLane) Submit(ctx context.Context, fn func(context.Context) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fn(ctx)
}
