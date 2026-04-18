package input

import (
	"context"
	"fmt"
	"sync"
	"time"

	gatewayevents "github.com/1024XEngineer/anyclaw/pkg/gateway/events"
)

type ChannelID string

type ChannelStatus string

const (
	ChannelStatusConnected    ChannelStatus = "connected"
	ChannelStatusDisconnected ChannelStatus = "disconnected"
	ChannelStatusConnecting   ChannelStatus = "connecting"
	ChannelStatusError        ChannelStatus = "error"
)

type Message struct {
	ID        string                 `json:"id"`
	ChannelID ChannelID              `json:"channel_id"`
	SenderID  string                 `json:"sender_id"`
	Content   string                 `json:"content"`
	Type      MessageType            `json:"type"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

type MessageType string

const (
	MessageTypeText     MessageType = "text"
	MessageTypeImage    MessageType = "image"
	MessageTypeFile     MessageType = "file"
	MessageTypeCommand  MessageType = "command"
	MessageTypeResponse MessageType = "response"
)

type ChannelPlugin interface {
	ID() ChannelID
	Name() string
	Description() string
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	SendMessage(ctx context.Context, message *Message) error
	GetStatus() ChannelStatus
	GetCapabilities() ChannelCapabilities
	SetMessageHandler(handler MessageHandler)
	HealthCheck(ctx context.Context) error
}

type ChannelCapabilities struct {
	SupportsText     bool
	SupportsImages   bool
	SupportsFiles    bool
	SupportsCommands bool
	SupportsRichText bool
	SupportsMarkdown bool
	MaxMessageSize   int
}

type MessageHandler func(ctx context.Context, message *Message) error

type ChannelManager struct {
	mu           sync.RWMutex
	plugins      map[ChannelID]ChannelPlugin
	handlers     map[ChannelID]MessageHandler
	eventBus     *gatewayevents.EventBus
	healthTicker *time.Ticker
	stopHealth   chan struct{}
}

func NewChannelManager(eventBus *gatewayevents.EventBus) *ChannelManager {
	return &ChannelManager{
		plugins:    make(map[ChannelID]ChannelPlugin),
		handlers:   make(map[ChannelID]MessageHandler),
		eventBus:   eventBus,
		stopHealth: make(chan struct{}),
	}
}

func (m *ChannelManager) RegisterPlugin(plugin ChannelPlugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := plugin.ID()
	if _, exists := m.plugins[id]; exists {
		return fmt.Errorf("channel plugin %s already registered", id)
	}

	m.plugins[id] = plugin
	plugin.SetMessageHandler(func(ctx context.Context, message *Message) error {
		return m.handleMessage(ctx, id, message)
	})

	if m.eventBus != nil {
		m.eventBus.PublishAsync(context.Background(), gatewayevents.Event{
			Type:   gatewayevents.EventChannelConnect,
			Source: "channel_manager",
			Data: map[string]interface{}{
				"channel_id":   string(id),
				"channel_name": plugin.Name(),
			},
			Timestamp: time.Now().Unix(),
		})
	}

	return nil
}

func (m *ChannelManager) UnregisterPlugin(id ChannelID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[id]
	if !exists {
		return fmt.Errorf("channel plugin %s not found", id)
	}

	if err := plugin.Disconnect(context.Background()); err != nil {
		return fmt.Errorf("failed to disconnect channel %s: %w", id, err)
	}

	delete(m.plugins, id)
	delete(m.handlers, id)
	return nil
}

func (m *ChannelManager) GetPlugin(id ChannelID) (ChannelPlugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[id]
	if !exists {
		return nil, fmt.Errorf("channel plugin %s not found", id)
	}

	return plugin, nil
}

func (m *ChannelManager) GetAllPlugins() []ChannelPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}

func (m *ChannelManager) ConnectAll(ctx context.Context) error {
	m.mu.RLock()
	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mu.RUnlock()

	var errors []error
	for _, plugin := range plugins {
		if err := plugin.Connect(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to connect channel %s: %w", plugin.ID(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to connect some channels: %v", errors)
	}

	return nil
}

func (m *ChannelManager) DisconnectAll(ctx context.Context) error {
	m.mu.RLock()
	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mu.RUnlock()

	var errors []error
	for _, plugin := range plugins {
		if err := plugin.Disconnect(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to disconnect channel %s: %w", plugin.ID(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to disconnect some channels: %v", errors)
	}

	return nil
}

func (m *ChannelManager) SendMessage(ctx context.Context, channelID ChannelID, message *Message) error {
	m.mu.RLock()
	plugin, exists := m.plugins[channelID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel plugin %s not found", channelID)
	}

	if plugin.GetStatus() != ChannelStatusConnected {
		return fmt.Errorf("channel %s is not connected", channelID)
	}

	if err := plugin.SendMessage(ctx, message); err != nil {
		if m.eventBus != nil {
			m.eventBus.PublishAsync(ctx, gatewayevents.Event{
				Type:   gatewayevents.EventChannelError,
				Source: "channel_manager",
				Data: map[string]interface{}{
					"channel_id": string(channelID),
					"error":      err.Error(),
				},
				Timestamp: time.Now().Unix(),
			})
		}
		return err
	}

	return nil
}

func (m *ChannelManager) BroadcastMessage(ctx context.Context, message *Message) error {
	m.mu.RLock()
	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		if plugin.GetStatus() == ChannelStatusConnected {
			plugins = append(plugins, plugin)
		}
	}
	m.mu.RUnlock()

	var errors []error
	for _, plugin := range plugins {
		msg := *message
		msg.ChannelID = plugin.ID()
		if err := plugin.SendMessage(ctx, &msg); err != nil {
			errors = append(errors, fmt.Errorf("failed to send to channel %s: %w", plugin.ID(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to broadcast to some channels: %v", errors)
	}

	return nil
}

func (m *ChannelManager) SetChannelHandler(channelID ChannelID, handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[channelID] = handler
}

func (m *ChannelManager) handleMessage(ctx context.Context, channelID ChannelID, message *Message) error {
	m.mu.RLock()
	handler, exists := m.handlers[channelID]
	m.mu.RUnlock()

	if !exists {
		return m.defaultHandler(ctx, message)
	}

	return handler(ctx, message)
}

func (m *ChannelManager) defaultHandler(ctx context.Context, message *Message) error {
	if m.eventBus != nil {
		m.eventBus.PublishAsync(ctx, gatewayevents.Event{
			Type:   gatewayevents.EventChannelMessage,
			Source: "channel_manager",
			Data: map[string]interface{}{
				"channel_id": string(message.ChannelID),
				"sender_id":  message.SenderID,
				"content":    message.Content,
				"type":       string(message.Type),
			},
			Timestamp: time.Now().Unix(),
		})
	}

	return nil
}

func (m *ChannelManager) StartHealthCheck(interval time.Duration) {
	m.mu.Lock()
	m.healthTicker = time.NewTicker(interval)
	m.mu.Unlock()

	go func() {
		for {
			select {
			case <-m.healthTicker.C:
				m.checkHealth()
			case <-m.stopHealth:
				return
			}
		}
	}()
}

func (m *ChannelManager) StopHealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthTicker != nil {
		m.healthTicker.Stop()
		close(m.stopHealth)
	}
}

func (m *ChannelManager) checkHealth() {
	m.mu.RLock()
	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mu.RUnlock()

	for _, plugin := range plugins {
		if err := plugin.HealthCheck(context.Background()); err != nil && m.eventBus != nil {
			m.eventBus.PublishAsync(context.Background(), gatewayevents.Event{
				Type:   gatewayevents.EventChannelError,
				Source: "channel_manager",
				Data: map[string]interface{}{
					"channel_id": string(plugin.ID()),
					"error":      err.Error(),
				},
				Timestamp: time.Now().Unix(),
			})
		}
	}
}

func (m *ChannelManager) GetStatus() map[ChannelID]ChannelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[ChannelID]ChannelStatus)
	for id, plugin := range m.plugins {
		status[id] = plugin.GetStatus()
	}

	return status
}
