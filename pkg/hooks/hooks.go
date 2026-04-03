package hooks

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Event string

const (
	EventStartup          Event = "startup"
	EventShutdown         Event = "shutdown"
	EventMessageIn        Event = "message.in"
	EventMessageOut       Event = "message.out"
	EventMessageSent      Event = "message.sent"
	EventToolCall         Event = "tool.call"
	EventToolResult       Event = "tool.result"
	EventToolError        Event = "tool.error"
	EventAgentStart       Event = "agent.start"
	EventAgentEnd         Event = "agent.end"
	EventAgentThink       Event = "agent.think"
	EventAgentError       Event = "agent.error"
	EventSessionCreate    Event = "session.create"
	EventSessionEnd       Event = "session.end"
	EventSessionMessage   Event = "session.message"
	EventCompactionBefore Event = "compaction.before"
	EventCompactionAfter  Event = "compaction.after"
	EventGatewayStart     Event = "gateway.start"
	EventGatewayStop      Event = "gateway.stop"
	EventError            Event = "error"
)

type HookFunc func(ctx context.Context, data interface{}) error

type HookResult struct {
	Continue bool
	Data     map[string]any
}

type InterceptingHookFunc func(ctx context.Context, data interface{}) (*HookResult, error)

type Hook struct {
	Name        string
	Event       Event
	Priority    int
	Fn          HookFunc
	InterceptFn InterceptingHookFunc
}

type Middleware func(next func() (*HookResult, error)) (*HookResult, error)

type Manager struct {
	mu    sync.RWMutex
	hooks map[Event][]Hook

	middleware []Middleware
}

func NewManager() *Manager {
	return &Manager{
		hooks: make(map[Event][]Hook),
	}
}

func (m *Manager) Use(mw ...Middleware) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.middleware = append(m.middleware, mw...)
}

func (m *Manager) Register(hook Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hooks := m.hooks[hook.Event]
	hooks = append(hooks, hook)

	for i := 0; i < len(hooks)-1; i++ {
		for j := i + 1; j < len(hooks); j++ {
			if hooks[j].Priority < hooks[i].Priority {
				hooks[i], hooks[j] = hooks[j], hooks[i]
			}
		}
	}

	m.hooks[hook.Event] = hooks
}

func (m *Manager) Emit(ctx context.Context, event Event, data interface{}) error {
	m.mu.RLock()
	hooks, ok := m.hooks[event]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	var lastErr error
	for _, hook := range hooks {
		if err := hook.Fn(ctx, data); err != nil {
			lastErr = fmt.Errorf("hook %s: %w", hook.Name, err)
		}
	}

	return lastErr
}

func (m *Manager) List(event Event) []Hook {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hooks[event]
}

type MessageData struct {
	Channel   string
	From      string
	To        string
	Text      string
	SessionID string
	Metadata  map[string]any
}

type ToolData struct {
	Name      string
	Arguments map[string]any
	SessionID string
	AgentID   string
}

type AgentData struct {
	AgentID   string
	Name      string
	SessionID string
	Result    string
}

type SessionData struct {
	SessionID string
	UserID    string
	Channel   string
}

func NewMessageHook(name string, priority int, fn func(ctx context.Context, msg *MessageData) error) Hook {
	return Hook{
		Name:     name,
		Event:    EventMessageIn,
		Priority: priority,
		Fn: func(ctx context.Context, data interface{}) error {
			if msg, ok := data.(*MessageData); ok {
				return fn(ctx, msg)
			}
			return nil
		},
	}
}

func NewToolHook(name string, priority int, fn func(ctx context.Context, tool *ToolData) error) Hook {
	return Hook{
		Name:     name,
		Event:    EventToolCall,
		Priority: priority,
		Fn: func(ctx context.Context, data interface{}) error {
			if tool, ok := data.(*ToolData); ok {
				return fn(ctx, tool)
			}
			return nil
		},
	}
}

func (m *Manager) EmitWithIntercept(ctx context.Context, event Event, data interface{}) (*HookResult, error) {
	m.mu.RLock()
	hooks, ok := m.hooks[event]
	middleware := m.middleware
	m.mu.RUnlock()

	if !ok {
		return &HookResult{Continue: true}, nil
	}

	for _, hook := range hooks {
		if hook.InterceptFn == nil {
			continue
		}

		hook := hook
		fn := func() (*HookResult, error) {
			return hook.InterceptFn(ctx, data)
		}

		wrapped := fn
		for i := len(middleware) - 1; i >= 0; i-- {
			mw := middleware[i]
			prev := wrapped
			wrapped = func() (*HookResult, error) {
				return mw(prev)
			}
		}

		result, err := wrapped()
		if err != nil {
			return result, fmt.Errorf("hook %s: %w", hook.Name, err)
		}
		if result != nil && !result.Continue {
			return result, nil
		}
	}
	return &HookResult{Continue: true}, nil
}

func NewInterceptingHook(name string, event Event, priority int, fn InterceptingHookFunc) Hook {
	return Hook{
		Name:        name,
		Event:       event,
		Priority:    priority,
		InterceptFn: fn,
	}
}

func NewOutboundMessageHook(name string, priority int, fn func(ctx context.Context, msg *MessageData) error) Hook {
	return Hook{
		Name:     name,
		Event:    EventMessageOut,
		Priority: priority,
		Fn: func(ctx context.Context, data interface{}) error {
			if msg, ok := data.(*MessageData); ok {
				return fn(ctx, msg)
			}
			return nil
		},
	}
}

func NewAgentThinkHook(name string, priority int, fn func(ctx context.Context, agent *AgentData) error) Hook {
	return Hook{
		Name:     name,
		Event:    EventAgentThink,
		Priority: priority,
		Fn: func(ctx context.Context, data interface{}) error {
			if agent, ok := data.(*AgentData); ok {
				return fn(ctx, agent)
			}
			return nil
		},
	}
}

func NewCompactionHook(name string, priority int, before bool, fn func(ctx context.Context, data map[string]any) error) Hook {
	event := EventCompactionAfter
	if before {
		event = EventCompactionBefore
	}
	return Hook{
		Name:     name,
		Event:    event,
		Priority: priority,
		Fn: func(ctx context.Context, data interface{}) error {
			if d, ok := data.(map[string]any); ok {
				return fn(ctx, d)
			}
			return nil
		},
	}
}

func LoggingMiddleware(next func() (*HookResult, error)) (*HookResult, error) {
	start := time.Now()
	result, err := next()
	_ = time.Since(start)
	return result, err
}

func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next func() (*HookResult, error)) (*HookResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		done := make(chan struct{})
		var result *HookResult
		var err error

		go func() {
			result, err = next()
			close(done)
		}()

		select {
		case <-done:
			return result, err
		case <-ctx.Done():
			return &HookResult{Continue: false}, fmt.Errorf("hook timed out after %v", timeout)
		}
	}
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, hooks := range m.hooks {
		count += len(hooks)
	}
	return count
}

func (m *Manager) Clear(event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.hooks, event)
}
