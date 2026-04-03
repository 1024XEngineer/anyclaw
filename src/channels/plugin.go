package channel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/event"
)

// ChannelID 渠道 ID
type ChannelID string

// ChannelStatus 渠道状态
type ChannelStatus string

const (
	ChannelStatusConnected    ChannelStatus = "connected"
	ChannelStatusDisconnected ChannelStatus = "disconnected"
	ChannelStatusConnecting   ChannelStatus = "connecting"
	ChannelStatusError        ChannelStatus = "error"
)

// Message 消息
type Message struct {
	ID        string                 `json:"id"`
	ChannelID ChannelID              `json:"channel_id"`
	SenderID  string                 `json:"sender_id"`
	Content   string                 `json:"content"`
	Type      MessageType            `json:"type"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// MessageType 消息类型
type MessageType string

const (
	MessageTypeText     MessageType = "text"
	MessageTypeImage    MessageType = "image"
	MessageTypeFile     MessageType = "file"
	MessageTypeCommand  MessageType = "command"
	MessageTypeResponse MessageType = "response"
)

// ChannelPlugin 渠道插件接口
type ChannelPlugin interface {
	// ID 返回渠道 ID
	ID() ChannelID

	// Name 返回渠道名称
	Name() string

	// Description 返回渠道描述
	Description() string

	// Connect 连接到渠道
	Connect(ctx context.Context) error

	// Disconnect 断开连接
	Disconnect(ctx context.Context) error

	// SendMessage 发送消息
	SendMessage(ctx context.Context, message *Message) error

	// GetStatus 获取渠道状态
	GetStatus() ChannelStatus

	// GetCapabilities 获取渠道能力
	GetCapabilities() ChannelCapabilities

	// SetMessageHandler 设置消息处理器
	SetMessageHandler(handler MessageHandler)

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// ChannelCapabilities 渠道能力
type ChannelCapabilities struct {
	SupportsText     bool
	SupportsImages   bool
	SupportsFiles    bool
	SupportsCommands bool
	SupportsRichText bool
	SupportsMarkdown bool
	MaxMessageSize   int
}

// MessageHandler 消息处理器
type MessageHandler func(ctx context.Context, message *Message) error

// ChannelManager 渠道管理器
type ChannelManager struct {
	mu           sync.RWMutex
	plugins      map[ChannelID]ChannelPlugin
	handlers     map[ChannelID]MessageHandler
	eventBus     *event.EventBus
	healthTicker *time.Ticker
	stopHealth   chan struct{}
}

// NewChannelManager 创建渠道管理器
func NewChannelManager(eventBus *event.EventBus) *ChannelManager {
	return &ChannelManager{
		plugins:    make(map[ChannelID]ChannelPlugin),
		handlers:   make(map[ChannelID]MessageHandler),
		eventBus:   eventBus,
		stopHealth: make(chan struct{}),
	}
}

// RegisterPlugin 注册渠道插件
func (m *ChannelManager) RegisterPlugin(plugin ChannelPlugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := plugin.ID()
	if _, exists := m.plugins[id]; exists {
		return fmt.Errorf("channel plugin %s already registered", id)
	}

	m.plugins[id] = plugin

	// 设置消息处理器
	plugin.SetMessageHandler(func(ctx context.Context, message *Message) error {
		return m.handleMessage(ctx, id, message)
	})

	// 发布注册事件
	if m.eventBus != nil {
		m.eventBus.PublishAsync(context.Background(), event.Event{
			Type:   event.EventChannelConnect,
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

// UnregisterPlugin 注销渠道插件
func (m *ChannelManager) UnregisterPlugin(id ChannelID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[id]
	if !exists {
		return fmt.Errorf("channel plugin %s not found", id)
	}

	// 断开连接
	if err := plugin.Disconnect(context.Background()); err != nil {
		return fmt.Errorf("failed to disconnect channel %s: %w", id, err)
	}

	delete(m.plugins, id)
	delete(m.handlers, id)

	return nil
}

// GetPlugin 获取渠道插件
func (m *ChannelManager) GetPlugin(id ChannelID) (ChannelPlugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[id]
	if !exists {
		return nil, fmt.Errorf("channel plugin %s not found", id)
	}

	return plugin, nil
}

// GetAllPlugins 获取所有渠道插件
func (m *ChannelManager) GetAllPlugins() []ChannelPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}

// ConnectAll 连接所有渠道
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

// DisconnectAll 断开所有渠道
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

// SendMessage 发送消息到指定渠道
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
		// 发布错误事件
		if m.eventBus != nil {
			m.eventBus.PublishAsync(ctx, event.Event{
				Type:   event.EventChannelError,
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

// BroadcastMessage 广播消息到所有渠道
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

// SetChannelHandler 设置渠道消息处理器
func (m *ChannelManager) SetChannelHandler(channelID ChannelID, handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handlers[channelID] = handler
}

// handleMessage 处理消息
func (m *ChannelManager) handleMessage(ctx context.Context, channelID ChannelID, message *Message) error {
	m.mu.RLock()
	handler, exists := m.handlers[channelID]
	m.mu.RUnlock()

	if !exists {
		// 使用默认处理器
		return m.defaultHandler(ctx, message)
	}

	return handler(ctx, message)
}

// defaultHandler 默认消息处理器
func (m *ChannelManager) defaultHandler(ctx context.Context, message *Message) error {
	// 发布消息事件
	if m.eventBus != nil {
		m.eventBus.PublishAsync(ctx, event.Event{
			Type:   event.EventChannelMessage,
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

// StartHealthCheck 启动健康检查
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

// StopHealthCheck 停止健康检查
func (m *ChannelManager) StopHealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthTicker != nil {
		m.healthTicker.Stop()
		close(m.stopHealth)
	}
}

// checkHealth 检查所有渠道健康状态
func (m *ChannelManager) checkHealth() {
	m.mu.RLock()
	plugins := make([]ChannelPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mu.RUnlock()

	for _, plugin := range plugins {
		if err := plugin.HealthCheck(context.Background()); err != nil {
			// 发布错误事件
			if m.eventBus != nil {
				m.eventBus.PublishAsync(context.Background(), event.Event{
					Type:   event.EventChannelError,
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
}

// GetStatus 获取所有渠道状态
func (m *ChannelManager) GetStatus() map[ChannelID]ChannelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[ChannelID]ChannelStatus)
	for id, plugin := range m.plugins {
		status[id] = plugin.GetStatus()
	}

	return status
}
