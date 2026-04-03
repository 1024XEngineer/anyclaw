package contextengine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Engine struct {
	mu       sync.RWMutex
	contexts map[string]*Context
	maxAge   time.Duration
}

type Context struct {
	ID        string
	Key       string
	Value     any
	CreatedAt time.Time
	ExpiresAt time.Time
	Metadata  map[string]string
}

type ContextConfig struct {
	MaxAge          time.Duration
	MaxSize         int
	AutoExpire      bool
	CleanupInterval time.Duration
}

func New(cfg ContextConfig) *Engine {
	engine := &Engine{
		contexts: make(map[string]*Context),
		maxAge:   cfg.MaxAge,
	}
	if cfg.AutoExpire && cfg.CleanupInterval > 0 {
		go engine.cleanup(cfg.CleanupInterval)
	}
	return engine
}

func (e *Engine) Set(ctx context.Context, key string, value any) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.contexts[key] = &Context{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		Key:       key,
		Value:     value,
		CreatedAt: now,
		ExpiresAt: now.Add(e.maxAge),
		Metadata:  make(map[string]string),
	}
	return nil
}

func (e *Engine) Get(ctx context.Context, key string) (any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if ctx, ok := e.contexts[key]; ok {
		if time.Now().Before(ctx.ExpiresAt) {
			return ctx.Value, nil
		}
		return nil, fmt.Errorf("context expired: %s", key)
	}
	return nil, fmt.Errorf("context not found: %s", key)
}

func (e *Engine) Delete(ctx context.Context, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.contexts, key)
	return nil
}

func (e *Engine) List() []*Context {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*Context, 0, len(e.contexts))
	for _, ctx := range e.contexts {
		result = append(result, ctx)
	}
	return result
}

func (e *Engine) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		e.mu.Lock()
		now := time.Now()
		for key, ctx := range e.contexts {
			if now.After(ctx.ExpiresAt) {
				delete(e.contexts, key)
			}
		}
		e.mu.Unlock()
	}
}

func (e *Engine) Size() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.contexts)
}
