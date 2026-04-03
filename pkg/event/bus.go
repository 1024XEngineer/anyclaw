package event

import (
	"context"
	"fmt"
	"sync"
)

// EventType 定义事件类型
type EventType string

const (
	// Agent 事件
	EventAgentStart    EventType = "agent.start"
	EventAgentStop     EventType = "agent.stop"
	EventAgentError    EventType = "agent.error"
	EventAgentThinking EventType = "agent.thinking"

	// Tool 事件
	EventToolCall   EventType = "tool.call"
	EventToolResult EventType = "tool.result"
	EventToolError  EventType = "tool.error"

	// Channel 事件
	EventChannelConnect    EventType = "channel.connect"
	EventChannelDisconnect EventType = "channel.disconnect"
	EventChannelMessage    EventType = "channel.message"
	EventChannelError      EventType = "channel.error"

	// Session 事件
	EventSessionCreate  EventType = "session.create"
	EventSessionClose   EventType = "session.close"
	EventSessionMessage EventType = "session.message"

	// Config 事件
	EventConfigLoad  EventType = "config.load"
	EventConfigSave  EventType = "config.save"
	EventConfigError EventType = "config.error"

	// Gateway 事件
	EventGatewayStart EventType = "gateway.start"
	EventGatewayStop  EventType = "gateway.stop"
	EventGatewayError EventType = "gateway.error"
)

// Event 表示一个事件
type Event struct {
	Type      EventType
	Source    string
	Data      map[string]interface{}
	Timestamp int64
}

// EventHandler 事件处理器
type EventHandler func(ctx context.Context, event Event) error

// EventMiddleware 事件中间件
type EventMiddleware func(next EventHandler) EventHandler

// EventBus 事件总线
type EventBus struct {
	mu         sync.RWMutex
	handlers   map[EventType][]EventHandler
	middleware []EventMiddleware
	history    []Event
	maxHistory int
}

// NewEventBus 创建新的事件总线
func NewEventBus() *EventBus {
	return &EventBus{
		handlers:   make(map[EventType][]EventHandler),
		middleware: make([]EventMiddleware, 0),
		history:    make([]Event, 0),
		maxHistory: 1000,
	}
}

// Subscribe 订阅事件
func (b *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// SubscribeAll 订阅所有事件
func (b *EventBus) SubscribeAll(handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 订阅所有事件类型
	allTypes := []EventType{
		EventAgentStart, EventAgentStop, EventAgentError, EventAgentThinking,
		EventToolCall, EventToolResult, EventToolError,
		EventChannelConnect, EventChannelDisconnect, EventChannelMessage, EventChannelError,
		EventSessionCreate, EventSessionClose, EventSessionMessage,
		EventConfigLoad, EventConfigSave, EventConfigError,
		EventGatewayStart, EventGatewayStop, EventGatewayError,
	}

	for _, eventType := range allTypes {
		b.handlers[eventType] = append(b.handlers[eventType], handler)
	}
}

// Use 添加中间件
func (b *EventBus) Use(middleware EventMiddleware) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.middleware = append(b.middleware, middleware)
}

// Publish 发布事件
func (b *EventBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	middleware := b.middleware
	b.mu.RUnlock()

	// 保存到历史
	b.mu.Lock()
	b.history = append(b.history, event)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
	b.mu.Unlock()

	// 如果没有处理器，返回 nil
	if len(handlers) == 0 {
		return nil
	}

	// 构建处理链
	var finalHandler EventHandler = func(ctx context.Context, event Event) error {
		var lastErr error
		for _, handler := range handlers {
			if err := handler(ctx, event); err != nil {
				lastErr = err
				// 继续执行其他处理器
			}
		}
		return lastErr
	}

	// 应用中间件
	for i := len(middleware) - 1; i >= 0; i-- {
		finalHandler = middleware[i](finalHandler)
	}

	return finalHandler(ctx, event)
}

// PublishAsync 异步发布事件
func (b *EventBus) PublishAsync(ctx context.Context, event Event) {
	go func() {
		if err := b.Publish(ctx, event); err != nil {
			// 记录错误但不阻塞
			fmt.Printf("event publish error: %v\n", err)
		}
	}()
}

// GetHistory 获取事件历史
func (b *EventBus) GetHistory(limit int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit <= 0 || limit > len(b.history) {
		limit = len(b.history)
	}

	start := len(b.history) - limit
	if start < 0 {
		start = 0
	}

	result := make([]Event, limit)
	copy(result, b.history[start:])
	return result
}

// Clear 清除所有订阅
func (b *EventBus) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers = make(map[EventType][]EventHandler)
	b.middleware = make([]EventMiddleware, 0)
}

// GetSubscriptions 获取订阅数量
func (b *EventBus) GetSubscriptions() map[EventType]int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[EventType]int)
	for eventType, handlers := range b.handlers {
		result[eventType] = len(handlers)
	}
	return result
}
