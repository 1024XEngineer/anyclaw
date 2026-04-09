package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"anyclaw/bus"
	"anyclaw/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

// GoogleChatChannel Google Chat 通道
type GoogleChatChannel struct {
	*BaseChannelImpl
	service      *chat.Service
	projectID    string
	credentials  string
	httpClient   *http.Client
	serviceMutex sync.RWMutex
}

// GoogleChatConfig Google Chat 配置
type GoogleChatConfig struct {
	BaseChannelConfig
	ProjectID   string `mapstructure:"project_id" json:"project_id"`
	Credentials string `mapstructure:"credentials" json:"credentials"` // Service account credentials JSON
}

// NewGoogleChatChannel 创建 Google Chat 通道
func NewGoogleChatChannel(cfg GoogleChatConfig, bus *bus.MessageBus) (*GoogleChatChannel, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("google chat project_id is required")
	}

	if cfg.Credentials == "" {
		return nil, fmt.Errorf("google chat credentials are required")
	}

	return &GoogleChatChannel{
		BaseChannelImpl: NewBaseChannelImpl("googlechat", "default", cfg.BaseChannelConfig, bus),
		projectID:       cfg.ProjectID,
		credentials:     cfg.Credentials,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Start 启动 Google Chat 通道
func (c *GoogleChatChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Google Chat channel",
		zap.String("project_id", c.projectID),
	)

	// 初始化 Google Chat 服务
	if err := c.InitService(ctx); err != nil {
		logger.Warn("Failed to initialize Google Chat service, webhook mode only", zap.Error(err))
		// 不返回错误，允许在 webhook 模式下运行
	}

	// 启动健康检查
	go c.healthCheck(ctx)

	logger.Info("Google Chat channel started")

	return nil
}

// healthCheck 健康检查
func (c *GoogleChatChannel) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Google Chat health check stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Google Chat health check stopped")
			return
		case <-ticker.C:
			// Google Chat 使用 webhook，我们只能检查通道是否运行
			if !c.IsRunning() {
				logger.Warn("Google Chat channel is not running")
			}
		}
	}
}

// HandleWebhook 处理 Google Chat webhook (需要在外部 HTTP 端点调用)
func (c *GoogleChatChannel) HandleWebhook(ctx context.Context, event *chat.DeprecatedEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// 检查权限
	senderID := event.User.Name
	if !c.IsAllowed(senderID) {
		logger.Warn("Google Chat message from unauthorized sender",
			zap.String("sender_name", senderID),
		)
		return nil
	}

	// 处理命令
	if strings.HasPrefix(event.Message.Text, "/") {
		return c.handleCommand(ctx, event)
	}

	// 构建入站消息
	msg := &bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: senderID,
		ChatID:   event.Space.Name,
		Content:  event.Message.Text,
		Metadata: map[string]interface{}{
			"message_id": event.Message.Name,
			"user_name":  event.User.DisplayName,
			"space_name": event.Space.DisplayName,
		},
		Timestamp: time.Now(),
	}

	return c.PublishInbound(ctx, msg)
}

// handleCommand 处理命令
func (c *GoogleChatChannel) handleCommand(ctx context.Context, event *chat.DeprecatedEvent) error {
	command := event.Message.Text

	var responseText string
	switch command {
	case "/start":
		responseText = "👋 Welcome to anyclaw!\n\nI can help you with various tasks. Send /help to see available commands."
	case "/help":
		responseText = `🐾 anyclaw commands:

/start - Get started
/help - Show this help message

You can chat with me directly and I'll do my best to help!`
	case "/status":
		responseText = fmt.Sprintf("✅ anyclaw is running\n\nChannel status: %s", map[bool]string{true: "🟢 Online", false: "🔴 Offline"}[c.IsRunning()])
	default:
		return nil
	}

	// 发送响应
	return c.Send(&bus.OutboundMessage{
		ChatID:    event.Space.Name,
		Content:   responseText,
		Timestamp: time.Now(),
	})
}

// Send 发送消息
func (c *GoogleChatChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("google chat channel is not running")
	}

	// 优先使用 webhook URL 发送
	if webhookURL, ok := msg.Metadata["webhookUrl"].(string); ok && webhookURL != "" {
		return c.SendWithWebhook(webhookURL, msg)
	}

	// 如果没有 webhook URL，使用 Google Chat API 发送
	c.serviceMutex.RLock()
	service := c.service
	c.serviceMutex.RUnlock()

	if service == nil {
		return fmt.Errorf("google chat service is not initialized, please provide webhookUrl in message metadata")
	}

	// 创建消息
	chatMsg := &chat.Message{
		Text: msg.Content,
	}

	// 获取 space 名称 (chatID)
	spaceName := msg.ChatID
	if spaceName == "" {
		return fmt.Errorf("chatID (space name) is required")
	}

	// 发送消息
	_, err := service.Spaces.Messages.Create(spaceName, chatMsg).Do()
	if err != nil {
		return fmt.Errorf("failed to send google chat message: %w", err)
	}

	logger.Info("Google Chat message sent via API",
		zap.String("space_name", spaceName),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// SendWithWebhook 使用 webhook 发送消息 (推荐方式)
func (c *GoogleChatChannel) SendWithWebhook(webhookURL string, msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("google chat channel is not running")
	}

	// 创建消息体
	payload := map[string]interface{}{
		"text": msg.Content,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 使用 HTTP 发送到 webhook
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	logger.Info("Google Chat webhook message sent",
		zap.String("webhook_url", webhookURL),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// Stop 停止 Google Chat 通道
func (c *GoogleChatChannel) Stop() error {
	c.serviceMutex.Lock()
	c.service = nil
	c.serviceMutex.Unlock()
	return c.BaseChannelImpl.Stop()
}

// InitService 初始化 Google Chat 服务 (如果需要主动发送)
func (c *GoogleChatChannel) InitService(ctx context.Context) error {
	c.serviceMutex.Lock()
	defer c.serviceMutex.Unlock()

	// 如果已经初始化，直接返回
	if c.service != nil {
		return nil
	}

	service, err := chat.NewService(ctx, option.WithCredentialsJSON([]byte(c.credentials)))
	if err != nil {
		return fmt.Errorf("failed to create google chat service: %w", err)
	}

	c.service = service
	logger.Info("Google Chat service initialized successfully")
	return nil
}
