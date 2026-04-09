package channels

import (
	"context"
	"fmt"
	"sync"
	"time"

	"anyclaw/bus"
	"anyclaw/config"
	"anyclaw/internal/logger"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"go.uber.org/zap"
)

// DingTalkChannel DingTalk 通道实现
type DingTalkChannel struct {
	*BaseChannelImpl
	config       config.DingTalkChannelConfig
	clientID     string
	clientSecret string
	streamClient *client.StreamClient
	ctx          context.Context
	cancel       context.CancelFunc
	// Map to store session webhooks for each chat
	sessionWebhooks sync.Map // chatID -> sessionWebhook
}

// NewDingTalkChannel 创建 DingTalk 通道实例
func NewDingTalkChannel(cfg config.DingTalkChannelConfig, bus *bus.MessageBus) (*DingTalkChannel, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("dingtalk client_id and client_secret are required")
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	return &DingTalkChannel{
		BaseChannelImpl: NewBaseChannelImpl("dingtalk", "default", baseCfg, bus),
		config:          cfg,
		clientID:        cfg.ClientID,
		clientSecret:    cfg.ClientSecret,
	}, nil
}

// Start 启动 DingTalk 通道 (Stream Mode)
func (c *DingTalkChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting DingTalk channel (Stream Mode)...",
		zap.String("client_id", c.clientID))

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create credential config
	cred := client.NewAppCredentialConfig(c.clientID, c.clientSecret)

	// Create stream client with options
	c.streamClient = client.NewStreamClient(
		client.WithAppCredential(cred),
		client.WithAutoReconnect(true),
	)

	// Register chatbot callback handler
	c.streamClient.RegisterChatBotCallbackRouter(c.onChatBotMessageReceived)

	// Start stream client
	if err := c.streamClient.Start(c.ctx); err != nil {
		return fmt.Errorf("failed to start stream client: %w", err)
	}

	logger.Info("DingTalk channel started (Stream Mode)")
	return nil
}

// Stop 停止 DingTalk 通道
func (c *DingTalkChannel) Stop() error {
	logger.Info("Stopping DingTalk channel...")

	if c.cancel != nil {
		c.cancel()
	}

	if c.streamClient != nil {
		c.streamClient.Close()
	}

	if err := c.BaseChannelImpl.Stop(); err != nil {
		return err
	}

	logger.Info("DingTalk channel stopped")
	return nil
}

// Send 发送消息到 DingTalk
func (c *DingTalkChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("dingtalk channel not running")
	}

	// Get session webhook from storage
	sessionWebhookRaw, ok := c.sessionWebhooks.Load(msg.ChatID)
	if !ok {
		return fmt.Errorf("no session_webhook found for chat %s, cannot send message", msg.ChatID)
	}

	sessionWebhook, ok := sessionWebhookRaw.(string)
	if !ok {
		return fmt.Errorf("invalid session_webhook type for chat %s", msg.ChatID)
	}

	logger.Info("DingTalk message to send",
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)))

	// Use session webhook to send reply
	return c.SendDirectReply(sessionWebhook, msg.Content)
}

// SendStream 发送流式消息 (DingTalk 不支持，收集后一次性发送)
func (c *DingTalkChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	var content string

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		if !msg.IsThinking && !msg.IsFinal {
			content += msg.Content
		}

		if msg.IsComplete {
			// Send complete message
			outMsg := &bus.OutboundMessage{
				Channel:   c.Name(),
				ChatID:    chatID,
				Content:   content,
				Timestamp: time.Now(),
			}
			return c.Send(outMsg)
		}
	}

	return nil
}

// onChatBotMessageReceived 处理 DingTalk 机器人消息
func (c *DingTalkChannel) onChatBotMessageReceived(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
	// Extract message content from Text field
	content := data.Text.Content
	if content == "" {
		// Try to extract from Content interface{} if Text is empty
		if contentMap, ok := data.Content.(map[string]interface{}); ok {
			if textContent, ok := contentMap["content"].(string); ok {
				content = textContent
			}
		}
	}

	if content == "" {
		return nil, nil // Ignore empty messages
	}

	senderID := data.SenderStaffId
	senderNick := data.SenderNick
	chatID := senderID
	if data.ConversationType != "1" {
		// For group chats (ConversationType: "1" = private, "2" = group)
		chatID = data.ConversationId
	}

	// Check if sender is allowed
	if !c.IsAllowed(senderID) {
		logger.Debug("DingTalk message from unauthorized sender, ignoring",
			zap.String("sender_id", senderID),
			zap.String("sender_nick", senderNick))
		return nil, nil
	}

	// Store session webhook for this chat so we can reply later
	c.sessionWebhooks.Store(chatID, data.SessionWebhook)

	logger.Info("DingTalk message received",
		zap.String("sender_nick", senderNick),
		zap.String("sender_id", senderID),
		zap.String("chat_id", chatID),
		zap.String("conversation_type", data.ConversationType),
		zap.Int("content_length", len(content)))

	// Build inbound message
	msg := &bus.InboundMessage{
		Content:   content,
		SenderID:  senderID,
		ChatID:    chatID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"sender_name":       senderNick,
			"conversation_id":   data.ConversationId,
			"conversation_type": data.ConversationType,
			"platform":          "dingtalk",
			"session_webhook":   data.SessionWebhook,
		},
	}

	// Publish inbound message
	_ = c.PublishInbound(ctx, msg)

	// Return nil to indicate we've handled message asynchronously
	return nil, nil
}

// SendDirectReply 使用 session webhook 发送直接回复
func (c *DingTalkChannel) SendDirectReply(sessionWebhook, content string) error {
	replier := chatbot.NewChatbotReplier()

	// Convert string content to []byte for API
	contentBytes := []byte(content)
	titleBytes := []byte("AnyClaw")

	// Send markdown formatted reply
	err := replier.SimpleReplyMarkdown(
		context.Background(),
		sessionWebhook,
		titleBytes,
		contentBytes,
	)

	if err != nil {
		logger.Error("Failed to send DingTalk reply",
			zap.Error(err))
		return fmt.Errorf("failed to send reply: %w", err)
	}

	return nil
}
