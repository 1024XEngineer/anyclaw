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

func (e *Engine) Set(_ context.Context, key string, value any) error {
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

func (e *Engine) Get(_ context.Context, key string) (any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if stored, ok := e.contexts[key]; ok {
		if time.Now().Before(stored.ExpiresAt) {
			return stored.Value, nil
		}
		return nil, fmt.Errorf("context expired: %s", key)
	}
	return nil, fmt.Errorf("context not found: %s", key)
}

func (e *Engine) Delete(_ context.Context, key string) error {
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
		result = append(result, cloneContext(ctx))
	}
	return result
}

func (e *Engine) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

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

func cloneContext(ctx *Context) *Context {
	if ctx == nil {
		return nil
	}

	cloned := *ctx
	if ctx.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(ctx.Metadata))
		for key, value := range ctx.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}
