package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"anyclaw/bus"
	"anyclaw/config"
	"anyclaw/internal/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WeWorkWsBotChannel 企业微信 WebSocket 机器人通道
type WeWorkWsBotChannel struct {
	*BaseChannelImpl
	config              config.WeWorkWsBotChannelConfig
	conn                *websocket.Conn
	ctx                 context.Context
	cancel              context.CancelFunc
	connMu              sync.Mutex
	waitResponseMsg     map[string]weworkWsBotMsgInfo
	waitResponseMsgLock sync.RWMutex
	connected           bool
	stopChan            chan struct{}
}

// NewWeWorkWsBotChannel 创建企业微信 WebSocket 机器人通道
func NewWeWorkWsBotChannel(cfg config.WeWorkWsBotChannelConfig, messageBus *bus.MessageBus) (*WeWorkWsBotChannel, error) {
	if cfg.BotID == "" || cfg.SecretID == "" {
		return nil, fmt.Errorf("wework websocket bot_id and secret_id is required")
	}

	if cfg.URL == "" {
		cfg.URL = "wss://openws.work.weixin.qq.com"
	}

	if cfg.ReconnectDelay == 0 {
		cfg.ReconnectDelay = 3
	}

	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 30
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AccountID:  cfg.BotID,
		Name:       "wework_wsbot",
		AllowedIDs: cfg.AllowedIDs,
	}

	channel := &WeWorkWsBotChannel{
		BaseChannelImpl: NewBaseChannelImpl("wework_wsbot", cfg.BotID, baseCfg, messageBus),
		config:          cfg,
		stopChan:        make(chan struct{}),
		waitResponseMsg: make(map[string]weworkWsBotMsgInfo),
	}

	return channel, nil
}

// Start 启动通道
func (c *WeWorkWsBotChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	logger.Info("Starting WeWork WebSocket bot channel", zap.String("url", c.config.URL))

	if err := c.doConnect(); err != nil {
		return fmt.Errorf("websocket connect failed: %w", err)
	}

	// 启动消息处理循环
	go c.handleMessages()

	return nil
}

// doConnect 执行实际的连接
func (c *WeWorkWsBotChannel) doConnect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(c.ctx, c.config.URL, c.config.Header)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.conn = conn
	c.connected = true

	logger.Info("WebSocket connected", zap.String("url", c.config.URL))

	subscribe := weworkWsBotRequest[map[string]string]{
		Cmd: "aibot_subscribe",
		Header: map[string]string{
			"req_id": uuid.New().String(),
		},
		Body: map[string]string{
			"bot_id": c.config.BotID,
			"secret": c.config.SecretID,
		},
	}

	return conn.WriteJSON(subscribe)
}

// handleMessages 处理消息循环
func (c *WeWorkWsBotChannel) handleMessages() {
	defer func() {
		c.connMu.Lock()
		c.connected = false
		c.connMu.Unlock()
		logger.Warn("WebSocket disconnected", zap.String("channel", c.Name()))
	}()

	// 心跳定时器
	heartbeatTicker := time.NewTicker(time.Duration(c.config.Heartbeat) * time.Second)
	defer heartbeatTicker.Stop()

	// 消息通道
	messageChan := make(chan []byte, 100)
	errorChan := make(chan error, 1)

	// 读取消息 goroutine
	go func() {
		for {
			c.connMu.Lock()
			conn := c.conn
			c.connMu.Unlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				errorChan <- err
				return
			}
			messageChan <- message
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-heartbeatTicker.C:
			c.sendHeartbeat()
		case message := <-messageChan:
			c.handleMessage(message)
		case err := <-errorChan:
			logger.Warn("WebSocket read error", zap.Error(err))

			// 自动重连
			if c.config.Reconnect && c.ctx.Err() == nil {
				c.handleReconnect(err)
				return
			}
			return
		}
	}
}

// handleReconnect 处理重连
func (c *WeWorkWsBotChannel) handleReconnect(lastErr error) {
	logger.Info("WebSocket reconnecting", zap.Error(lastErr))

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-time.After(time.Second * time.Duration(c.config.ReconnectDelay)):
			if err := c.doConnect(); err != nil {
				logger.Warn("WebSocket reconnection failed", zap.Error(err))
				continue
			}

			// 重新启动消息处理
			go c.handleMessages()

			logger.Info("WebSocket reconnected")
			return
		}
	}
}

// sendHeartbeat 发送心跳
func (c *WeWorkWsBotChannel) sendHeartbeat() {
	c.connMu.Lock()
	conn := c.conn
	if conn != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("{\"cmd\":\"ping\",\"headers\":{\"req_id\":\"%s\"}}", uuid.New().String())))
	}
	c.connMu.Unlock()
	// 移除过期的带待回复内容
	c.waitResponseMsgLock.Lock()
	var removeKey []string
	tnow := time.Now().Unix()
	for k, v := range c.waitResponseMsg {
		if v.MsgTime == 0 || tnow-v.MsgTime > 24*60*60 { //超过24小时的消息忽略，微信官方规定收到消息须在24小时内回复，否则要用主动推送
			removeKey = append(removeKey, k)
		}
	}
	for _, k := range removeKey {
		delete(c.waitResponseMsg, k)
	}
	defer c.waitResponseMsgLock.Unlock()
}

// Stop 停止通道
func (c *WeWorkWsBotChannel) Stop() error {
	close(c.stopChan)

	if c.cancel != nil {
		c.cancel()
	}

	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.connected = false
	c.connMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	return c.BaseChannelImpl.Stop()
}

// Send 发送消息
func (c *WeWorkWsBotChannel) Send(msg *bus.OutboundMessage) error {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("websocket not connected")
	}
	c.waitResponseMsgLock.RLock()
	v := c.waitResponseMsg[msg.ReplyTo]
	c.waitResponseMsgLock.RUnlock()
	if v.MsgTime > 0 { //对于收到的消息进行回复
		var resp weworkWsBotRequest[weworkWsBotMsgResponse]
		resp.Cmd = "aibot_respond_msg"
		resp.Header = map[string]string{
			"req_id": v.ReqID,
		}
		resp.Body.MsgType = "stream"
		resp.Body.Stream.ID = v.StreamID
		resp.Body.Stream.Finish = true
		resp.Body.Stream.Content = msg.Content
		return c.sendMessage(resp)
	}
	//没有收到消息，用主动推送
	var resp weworkWsBotRequest[weworkWsBotMsgPushData]
	resp.Cmd = "aibot_send_msg"
	resp.Header = map[string]string{
		"req_id": uuid.New().String(),
	}
	resp.Body.MsgType = "markdown"
	resp.Body.ChatID = msg.ChatID
	resp.Body.Markdown.Content = msg.Content
	return c.sendMessage(resp)
}

// SendStream 发送流式消息 (默认实现，收集所有chunk后一次性发送)
func (c *WeWorkWsBotChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	c.waitResponseMsgLock.RLock()
	v := c.waitResponseMsg[chatID]
	c.waitResponseMsgLock.RUnlock()
	var content strings.Builder
	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}
		if !msg.IsThinking && !msg.IsFinal {
			content.WriteString(msg.Content)
			if v.MsgTime > 0 { //对于收到的消息进行回复
				var resp weworkWsBotRequest[weworkWsBotMsgResponse]
				resp.Cmd = "aibot_respond_msg"
				resp.Header = map[string]string{
					"req_id": v.ReqID,
				}
				resp.Body.MsgType = "stream"
				resp.Body.Stream.ID = v.StreamID
				resp.Body.Stream.Finish = false
				resp.Body.Stream.Content = content.String()
				c.sendMessage(resp)
			}
		}
		if msg.IsComplete {
			// Send complete message - note: this needs actual channel implementation
			// Default implementation just collects content
			return nil
		}
	}
	return nil
}

// IsConnected 检查是否已连接
func (c *WeWorkWsBotChannel) IsConnected() bool {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.connected && c.conn != nil
}

func (c *WeWorkWsBotChannel) sendMessage(v any) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *WeWorkWsBotChannel) handleMessage(data []byte) {
	logger.Debug("Received WebSocket message", zap.ByteString("data", data))
	var resp weworkWsBotResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		logger.Warn("WebSocket unmarshal error", zap.Error(err), zap.ByteString("data", data))
		return
	}
	if resp.Cmd == "" {
		logger.Debug("Received WebSocket message no cmd", zap.ByteString("data", data))
		return
	}
	msg := resp.Body
	reqID := resp.Header["req_id"]
	c.waitResponseMsgLock.Lock()
	c.waitResponseMsg[msg.MsgID] = weworkWsBotMsgInfo{
		ReqID:      reqID,
		MsgTime:    time.Now().Unix(),
		FromUserID: msg.From.UserID,
		ChatID:     msg.ChatID,
		StreamID:   uuid.New().String(),
	}
	c.waitResponseMsgLock.Unlock()
	if msg.MsgType == "text" {
		inMsg := &bus.InboundMessage{
			ID:        msg.MsgID,
			Content:   msg.Text.Content,
			SenderID:  msg.From.UserID,
			ChatID:    msg.MsgID,
			Channel:   c.Name(),
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"req_id": reqID,
			},
		}
		_ = c.PublishInbound(context.Background(), inMsg)
	} else {
		chatID := msg.ChatID
		if chatID == "" {
			chatID = msg.From.UserID
		}
		outMsg := &bus.OutboundMessage{
			ID:        msg.MsgID,
			Content:   "不支持[" + msg.MsgType + "]类型消息",
			ChatID:    chatID,
			Channel:   c.Name(),
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"req_id": resp.Header["req_id"],
			},
		}
		c.Send(outMsg)
	}
}

type weworkWsBotMsgInfo struct {
	ReqID      string `json:"req_id"`
	MsgTime    int64  `json:"msg_time"`
	FromUserID string `json:"from_user_id"`
	ChatID     string `json:"chat_id"`   //当为群聊时有效
	StreamID   string `json:"stream_id"` //回复消息唯一ID
}

type weworkWsBotRequest[T any] struct {
	Cmd    string            `json:"cmd"`
	Header map[string]string `json:"headers"`
	Body   T                 `json:"body,omitempty"`
}

type weworkWsBotResponse struct {
	Cmd     string                  `json:"cmd"`
	Header  map[string]string       `json:"headers"`
	ErrCode int                     `json:"errcode"`
	ErrMsg  string                  `json:"errmsg"`
	Body    weworkWsBotResponseData `json:"body"`
}

type weworkWsBotResponseFrom struct {
	UserID string `json:"userid"`
}

type weworkWsBotResponseText struct {
	Content string `json:"content"`
}

type weworkWsBotResponseData struct {
	MsgID    string                  `json:"msgid"`
	AibotID  string                  `json:"aibotid"`
	ChatID   string                  `json:"chatid"`
	ChatType string                  `json:"chattype"`
	From     weworkWsBotResponseFrom `json:"from"`
	MsgType  string                  `json:"msgtype"`
	Text     weworkWsBotResponseText `json:"text"`
}

type weworkWsBotMsgResponse struct {
	MsgType string `json:"msgtype"`
	Stream  struct {
		ID      string `json:"id"`
		Finish  bool   `json:"finish"`
		Content string `json:"content"`
		MsgItem []struct {
			MsgType string `json:"msgtype"`
			Image   struct {
				Base64 string `json:"base64"`
				Md5    string `json:"md5"`
			} `json:"image"`
		} `json:"msg_item,omitempty"`
		Feedback struct {
			Id string `json:"id"`
		} `json:"feedback,omitempty"`
	} `json:"stream"`
}

type weworkWsBotMsgPushData struct {
	ChatID   string `json:"chatid"`
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}
