package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"anyclaw/bus"
	"anyclaw/internal/logger"
	"go.uber.org/zap"
)

const (
	// gotifyMaxRetries 最大重试次数
	gotifyMaxRetries = 3
	// gotifyRetryDelay 重试延迟
	gotifyRetryDelay = 2 * time.Second
)

// GotifyChannel Gotify 通道
type GotifyChannel struct {
	*BaseChannelImpl
	serverURL string
	appToken  string
	priority  int
	client    *http.Client
}

// GotifyConfig Gotify 配置
type GotifyConfig struct {
	BaseChannelConfig
	ServerURL string `mapstructure:"server_url" json:"server_url"`
	AppToken  string `mapstructure:"app_token" json:"app_token"`
	Priority  int    `mapstructure:"priority" json:"priority"` // 消息优先级 1-10
}

// NewGotifyChannel 创建 Gotify 通道
func NewGotifyChannel(accountID string, cfg GotifyConfig, bus *bus.MessageBus) (*GotifyChannel, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("gotify server_url is required")
	}
	if cfg.AppToken == "" {
		return nil, fmt.Errorf("gotify app_token is required")
	}

	// 标准化 server URL (确保以 / 结尾)
	serverURL := strings.TrimSuffix(cfg.ServerURL, "/") + "/"

	// 设置默认优先级
	priority := cfg.Priority
	if priority < 1 || priority > 10 {
		priority = 5
	}

	return &GotifyChannel{
		BaseChannelImpl: NewBaseChannelImpl("gotify", accountID, cfg.BaseChannelConfig, bus),
		serverURL:       serverURL,
		appToken:        cfg.AppToken,
		priority:        priority,
		// 配置 HTTP 客户端，使用连接池优化性能
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}, nil
}

// Start 启动 Gotify 通道
func (c *GotifyChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Gotify channel",
		zap.String("account_id", c.AccountID()),
		zap.String("server_url", c.serverURL),
	)

	// 验证连接，带重试机制
	if err := c.verifyConnectionWithRetry(ctx); err != nil {
		logger.Error("Failed to verify Gotify connection after retries",
			zap.Error(err),
			zap.Int("max_retries", gotifyMaxRetries))
		return err
	}

	return nil
}

// verifyConnectionWithRetry 验证与 Gotify 服务器的连接，带重试
func (c *GotifyChannel) verifyConnectionWithRetry(ctx context.Context) error {
	var lastErr error

	for i := 0; i < gotifyMaxRetries; i++ {
		if i > 0 {
			// 等待重试延迟
			select {
			case <-time.After(gotifyRetryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
			logger.Info("Retrying Gotify connection verification",
				zap.Int("attempt", i+1),
				zap.Int("max_retries", gotifyMaxRetries))
		}

		err := c.verifyConnection(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		// 如果是上下文取消，直接返回
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("connection verification failed after %d attempts: %w", gotifyMaxRetries, lastErr)
}

// verifyConnection 验证与 Gotify 服务器的连接
func (c *GotifyChannel) verifyConnection(ctx context.Context) error {
	// 尝试获取当前应用信息
	reqURL := c.serverURL + "current/application"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?token="+c.appToken, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to gotify server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gotify server returned status %d (app token may be invalid)", resp.StatusCode)
	}

	logger.Info("Gotify connection verified successfully")
	return nil
}

// Send 发送消息到 Gotify
func (c *GotifyChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("gotify channel is not running")
	}

	// 构建 Gotify 消息
	gotifyMsg := map[string]interface{}{
		"message":  msg.Content,
		"priority": c.priority,
	}

	// 如果消息中包含标题信息，使用它
	if title, ok := msg.Metadata["title"].(string); ok && title != "" {
		gotifyMsg["title"] = title
	} else {
		// 默认标题
		gotifyMsg["title"] = "AnyClaw Message"
	}

	jsonData, err := json.Marshal(gotifyMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 构建请求 URL (token 作为 query 参数)
	reqURL := c.serverURL + "message?token=" + url.QueryEscape(c.appToken)

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		logger.Error("Gotify server returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.Any("error", errResp))
		return fmt.Errorf("gotify server returned status %d", resp.StatusCode)
	}

	logger.Debug("Gotify message sent successfully",
		zap.String("account_id", c.AccountID()),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// SendStream 发送流式消息
// Gotify 是推送平台，不像聊天平台那样有多个聊天会话，因此 chatID 参数未使用
// 消息会收集所有内容后一次性发送到 Gotify
func (c *GotifyChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	var content strings.Builder
	var title string

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		// 收集标题
		if msgTitle, ok := msg.Metadata["title"].(string); ok && title == "" {
			title = msgTitle
		}

		// 收集非思考内容
		if !msg.IsThinking && !msg.IsFinal {
			content.WriteString(msg.Content)
		}

		if msg.IsComplete {
			// 发送完整消息
			outboundMsg := &bus.OutboundMessage{
				Channel:  c.Name(),
				ChatID:   chatID,
				Content:  content.String(),
				Metadata: map[string]interface{}{},
			}
			if title != "" {
				outboundMsg.Metadata["title"] = title
			}
			return c.Send(outboundMsg)
		}
	}

	return nil
}
