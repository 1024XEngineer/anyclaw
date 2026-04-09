package channels

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"anyclaw/bus"
	"anyclaw/internal/logger"
	"go.uber.org/zap"
)

// WeixinChannel is the channel implementation for Weixin
type WeixinChannel struct {
	*BaseChannelImpl
	apiClient     *WeixinAPIClient
	media         *WeixinMediaHandler
	auth          *WeixinAuth
	config        WeixinConfig
	contextTokens sync.Map // userID -> contextToken

	// Typing indicator management
	typingTicket   string
	typingTicketMu sync.RWMutex
}

// NewWeixinChannel creates a new Weixin channel
func NewWeixinChannel(accountID string, cfg WeixinConfig, messageBus *bus.MessageBus) (*WeixinChannel, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultWeixinBaseURL
	}
	if cfg.CDNBaseURL == "" {
		cfg.CDNBaseURL = DefaultWeixinCDNURL
	}

	apiClient := NewWeixinAPIClient(cfg.BaseURL, cfg.Token)
	media := NewWeixinMediaHandler(apiClient, cfg.CDNBaseURL)
	auth, err := NewWeixinAuth(apiClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth handler: %w", err)
	}

	return &WeixinChannel{
		BaseChannelImpl: NewBaseChannelImpl("weixin", accountID, cfg.BaseChannelConfig, messageBus),
		apiClient:       apiClient,
		media:           media,
		auth:            auth,
		config:          cfg,
	}, nil
}

// Start starts the Weixin channel
func (c *WeixinChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	// Load token from storage if not provided in config
	if c.config.Token == "" {
		tokenInfo, err := c.auth.LoadToken(c.AccountID())
		if err != nil {
			logger.Warn("Failed to load token from storage",
				zap.String("account_id", c.AccountID()),
				zap.Error(err))
		} else if tokenInfo != nil && c.auth.IsTokenValid(tokenInfo) {
			c.apiClient.SetToken(tokenInfo.Token)
			logger.Info("Loaded token from storage",
				zap.String("account_id", c.AccountID()),
				zap.String("ilink_bot_id", tokenInfo.ILinkBotID))
		} else if tokenInfo != nil {
			logger.Warn("Stored token has expired",
				zap.String("account_id", c.AccountID()))
		}
	}

	// Check if token is available
	if c.apiClient.GetToken() == "" {
		return fmt.Errorf("no token available, please login first using 'anyclaw channels weixin login %s'", c.AccountID())
	}

	// Get initial config (including typing ticket)
	go c.refreshConfig(ctx)

	// Start message receiver
	go c.receiveMessages(ctx)

	logger.Info("Weixin channel started",
		zap.String("account_id", c.AccountID()))

	return nil
}

// refreshConfig periodically refreshes the config
func (c *WeixinChannel) refreshConfig(ctx context.Context) {
	// Initial refresh
	if err := c.refreshTypingTicket(ctx); err != nil {
		logger.Warn("Failed to get initial config", zap.Error(err))
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.WaitForStop():
			return
		case <-ticker.C:
			if err := c.refreshTypingTicket(ctx); err != nil {
				logger.Warn("Failed to refresh config", zap.Error(err))
			}
		}
	}
}

// refreshTypingTicket refreshes the typing ticket
func (c *WeixinChannel) refreshTypingTicket(ctx context.Context) error {
	config, err := c.apiClient.GetConfig(ctx, "", "")
	if err != nil {
		return err
	}

	c.typingTicketMu.Lock()
	c.typingTicket = config.TypingTicket
	c.typingTicketMu.Unlock()

	return nil
}

// getTypingTicket returns the current typing ticket
func (c *WeixinChannel) getTypingTicket() string {
	c.typingTicketMu.RLock()
	defer c.typingTicketMu.RUnlock()
	return c.typingTicket
}

// receiveMessages handles the long polling loop for messages
func (c *WeixinChannel) receiveMessages(ctx context.Context) {
	var getUpdatesBuf string
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			logger.Info("Weixin channel stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Weixin channel stopped")
			return
		default:
			resp, err := c.apiClient.GetUpdates(ctx, &GetUpdatesReq{
				GetUpdatesBuf: getUpdatesBuf,
			})

			if err != nil {
				logger.Error("Failed to get updates",
					zap.Error(err),
					zap.Duration("backoff", backoff))

				// Backoff on error
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
					backoff = backoff * 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
					continue
				}
			}

			// Check for session timeout error
			if resp.ErrCode == -14 {
				logger.Error("Weixin session expired, need to re-login",
					zap.Int("errcode", resp.ErrCode))
				return
			}

			// Reset backoff on success
			backoff = time.Second

			// Update sync cursor
			if resp.GetUpdatesBuf != "" {
				getUpdatesBuf = resp.GetUpdatesBuf
			}

			// Handle messages
			for _, msg := range resp.Msgs {
				if err := c.handleInboundMessage(ctx, msg); err != nil {
					logger.Error("Failed to handle message",
						zap.Error(err),
						zap.Int64("message_id", msg.MessageID))
				}
			}
		}
	}
}

// handleInboundMessage processes an incoming message
func (c *WeixinChannel) handleInboundMessage(ctx context.Context, msg *WeixinMessage) error {
	// Check permission
	if !c.IsAllowed(msg.FromUserID) {
		logger.Warn("Message from unauthorized sender",
			zap.String("sender_id", msg.FromUserID))
		return nil
	}

	// Store context token for future responses
	if msg.ContextToken != "" {
		key := c.contextTokenKey(msg.FromUserID)
		c.contextTokens.Store(key, msg.ContextToken)
	}

	// Extract content and media
	content := c.extractContent(msg)
	media := c.extractMedia(msg)

	// Build inbound message
	inbound := &bus.InboundMessage{
		Channel:   c.Name(),
		AccountID: c.AccountID(),
		SenderID:  msg.FromUserID,
		ChatID:    msg.FromUserID, // Use sender ID as chat ID for 1:1 chats
		Content:   content,
		Media:     media,
		Metadata: map[string]interface{}{
			"message_id":    msg.MessageID,
			"session_id":    msg.SessionID,
			"message_type":  msg.MessageType,
			"create_time":   msg.CreateTimeMs,
			"context_token": msg.ContextToken,
		},
		Timestamp: time.Now(),
	}

	// Publish to bus
	return c.PublishInbound(ctx, inbound)
}

// extractContent extracts text content from a message
func (c *WeixinChannel) extractContent(msg *WeixinMessage) string {
	var parts []string

	for _, item := range msg.ItemList {
		if item.Type == MessageItemTypeText && item.TextItem != nil {
			parts = append(parts, item.TextItem.Text)
		}
	}

	return strings.Join(parts, "\n")
}

// extractMedia extracts media from a message
func (c *WeixinMediaHandler) extractMediaFromMessage(msg *WeixinMessage) []bus.Media {
	var media []bus.Media

	for _, item := range msg.ItemList {
		var m bus.Media

		switch item.Type {
		case MessageItemTypeImage:
			m.Type = "image"
			if item.ImageItem != nil && item.ImageItem.Media != nil {
				m.URL = item.ImageItem.URL
				// Store encrypt_query_param for later download
				m.Metadata = map[string]interface{}{
					"encrypt_query_param": item.ImageItem.Media.EncryptQueryParam,
					"aes_key":             item.ImageItem.Media.AESKey,
				}
			}
		case MessageItemTypeVoice:
			m.Type = "audio"
			if item.VoiceItem != nil && item.VoiceItem.Media != nil {
				m.Metadata = map[string]interface{}{
					"encrypt_query_param": item.VoiceItem.Media.EncryptQueryParam,
					"aes_key":             item.VoiceItem.Media.AESKey,
					"playtime":            item.VoiceItem.Playtime,
				}
			}
		case MessageItemTypeVideo:
			m.Type = "video"
			if item.VideoItem != nil && item.VideoItem.Media != nil {
				m.Metadata = map[string]interface{}{
					"encrypt_query_param": item.VideoItem.Media.EncryptQueryParam,
					"aes_key":             item.VideoItem.Media.AESKey,
					"play_length":         item.VideoItem.PlayLength,
				}
			}
		case MessageItemTypeFile:
			m.Type = "document"
			if item.FileItem != nil && item.FileItem.Media != nil {
				m.MimeType = "application/octet-stream"
				if item.FileItem.FileName != "" {
					m.Metadata = map[string]interface{}{
						"file_name":           item.FileItem.FileName,
						"encrypt_query_param": item.FileItem.Media.EncryptQueryParam,
						"aes_key":             item.FileItem.Media.AESKey,
					}
				}
			}
		default:
			continue
		}

		media = append(media, m)
	}

	return media
}

// extractMedia wraps the media handler function
func (c *WeixinChannel) extractMedia(msg *WeixinMessage) []bus.Media {
	return c.media.extractMediaFromMessage(msg)
}

// Send sends a message
func (c *WeixinChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("weixin channel is not running")
	}

	// Build message items
	var items []MessageItem

	// Add text content
	if msg.Content != "" {
		items = append(items, MessageItem{
			Type: MessageItemTypeText,
			TextItem: &TextItem{
				Text: msg.Content,
			},
		})
	}

	// Get context token
	contextToken := c.getContextToken(msg.ChatID)

	// Send message
	req := &SendMessageReq{
		ToUserID:     msg.ChatID,
		ContextToken: contextToken,
		ItemList:     items,
	}

	if err := c.apiClient.SendMessage(context.Background(), req); err != nil {
		// Check for session expired error
		if strings.Contains(err.Error(), "-14") {
			// Clear context token
			c.clearContextToken(msg.ChatID)
		}
		return fmt.Errorf("failed to send message: %w", err)
	}

	logger.Info("Weixin message sent",
		zap.String("chat_id", msg.ChatID))

	return nil
}

// SendStream sends streaming messages
func (c *WeixinChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("weixin channel is not running")
	}

	var content strings.Builder

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		if !msg.IsThinking && !msg.IsFinal {
			content.WriteString(msg.Content)
		}

		if msg.IsComplete {
			// Send the complete message
			return c.Send(&bus.OutboundMessage{
				Channel: "weixin",
				ChatID:  chatID,
				Content: content.String(),
			})
		}
	}

	// If stream closed without IsComplete, send what we have
	if content.Len() > 0 {
		return c.Send(&bus.OutboundMessage{
			Channel: "weixin",
			ChatID:  chatID,
			Content: content.String(),
		})
	}

	return nil
}

// contextTokenKey generates the key for storing context tokens
func (c *WeixinChannel) contextTokenKey(userID string) string {
	return c.AccountID() + ":" + userID
}

// getContextToken retrieves the context token for a user
func (c *WeixinChannel) getContextToken(userID string) string {
	key := c.contextTokenKey(userID)
	if v, ok := c.contextTokens.Load(key); ok {
		return v.(string)
	}
	return ""
}

// clearContextToken clears the context token for a user
func (c *WeixinChannel) clearContextToken(userID string) {
	key := c.contextTokenKey(userID)
	c.contextTokens.Delete(key)
}

// SetToken sets the authentication token
func (c *WeixinChannel) SetToken(token string) {
	c.apiClient.SetToken(token)
	c.config.Token = token
}

// GetAuth returns the auth handler for login operations
func (c *WeixinChannel) GetAuth() *WeixinAuth {
	return c.auth
}
