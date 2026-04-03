package hooks

import (
	"context"
	"fmt"
	"sync"
)

type Event string

const (
	EventStartup       Event = "startup"
	EventShutdown      Event = "shutdown"
	EventMessageIn     Event = "message.in"
	EventMessageOut    Event = "message.out"
	EventToolCall      Event = "tool.call"
	EventToolResult    Event = "tool.result"
	EventAgentStart    Event = "agent.start"
	EventAgentEnd      Event = "agent.end"
	EventSessionCreate Event = "session.create"
	EventSessionEnd    Event = "session.end"
	EventError         Event = "error"
)

type HookFunc func(ctx context.Context, data interface{}) error

type Hook struct {
	Name     string
	Event    Event
	Priority int
	Fn       HookFunc
}

type Manager struct {
	mu    sync.RWMutex
	hooks map[Event][]Hook
}

func NewManager() *Manager {
	return &Manager{
		hooks: make(map[Event][]Hook),
	}
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
