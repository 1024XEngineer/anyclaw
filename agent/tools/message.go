package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"anyclaw/bus"
	"anyclaw/internal/logger"
	"go.uber.org/zap"
)

// MessageTool 消息工具
type MessageTool struct {
	bus         *bus.MessageBus
	currentChan string
	currentChat string
}

// NewMessageTool 创建消息工具
func NewMessageTool(bus *bus.MessageBus) *MessageTool {
	return &MessageTool{
		bus: bus,
	}
}

// SetCurrent 设置当前通道和聊天
func (t *MessageTool) SetCurrent(channel, chatID string) {
	t.currentChan = channel
	t.currentChat = chatID
}

// SendMessage 发送消息
func (t *MessageTool) SendMessage(ctx context.Context, params map[string]interface{}) (string, error) {
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required")
	}

	// 过滤中间态错误和拒绝消息
	if isFilteredContent(content) {
		logger.Warn("Message tool send was filtered out",
			zap.Int("content_length", len(content)))
		// 返回成功但不实际发送消息
		return "Message was filtered and not sent", nil
	}

	// 获取目标通道
	channel := t.currentChan
	if ch, ok := params["channel"].(string); ok && ch != "" {
		channel = ch
	}

	chatID := t.currentChat
	if cid, ok := params["chat_id"].(string); ok && cid != "" {
		chatID = cid
	}

	if channel == "" || chatID == "" {
		return "", fmt.Errorf("channel and chat_id are required")
	}

	// 发送消息
	msg := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
	}

	if err := t.bus.PublishOutbound(ctx, msg); err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return fmt.Sprintf("Message sent to %s:%s", channel, chatID), nil
}

// isFilteredContent 检查内容是否应该被过滤
func isFilteredContent(content string) bool {
	if content == "" {
		return false
	}

	// 检测常见的 LLM 拒绝消息模式（中英文）
	rejectionPatterns := []string{
		"作为一个人工智能语言模型",
		"作为AI语言模型",
		"作为一个AI",
		"作为一个人工智能",
		"我还没有学习",
		"我还没学习",
		"我无法回答",
		"我不能回答",
		"I'm sorry, but I cannot",
		"As an AI language model",
		"As an AI assistant",
		"I cannot answer",
		"I'm not able to answer",
		"I cannot provide",
	}

	contentLower := strings.ToLower(content)
	for _, pattern := range rejectionPatterns {
		if strings.Contains(content, pattern) || strings.Contains(contentLower, strings.ToLower(pattern)) {
			return true
		}
	}

	// 检测中间态错误消息（包含 "An unknown error occurred" 的）
	if strings.Contains(content, "An unknown error occurred") {
		return true
	}

	// 检测工具执行失败的消息（这些应该被 LLM 处理后返回更好的答案）
	if strings.Contains(content, "工具执行失败") ||
		strings.Contains(content, "Tool execution failed") ||
		(strings.Contains(content, "## ") && strings.Contains(content, "**错误**")) {
		return true
	}

	// 检测纯技术错误消息
	techErrorPatterns := []string{
		"context deadline exceeded",
		"context canceled",
		"connection refused",
		"network error",
	}

	for _, pattern := range techErrorPatterns {
		if strings.Contains(contentLower, pattern) {
			return true
		}
	}

	return false
}

// GetTools 获取所有消息工具
func (t *MessageTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"message",
			"Send a message to a chat channel",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Message content to send",
					},
					"channel": map[string]interface{}{
						"type":        "string",
						"description": "Target channel (default: current)",
					},
					"chat_id": map[string]interface{}{
						"type":        "string",
						"description": "Target chat ID (default: current)",
					},
				},
				"required": []string{"content"},
			},
			t.SendMessage,
		),
	}
}
