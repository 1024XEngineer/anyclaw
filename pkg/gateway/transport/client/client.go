package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type GatewayClient struct {
	addr       string
	wsConn     *websocket.Conn
	httpClient *http.Client
	mu         sync.RWMutex

	onMessage  func(msg IncomingMessage)
	onToolCall func(call ToolCall)
	onPresence func(presence Presence)
	onTyping   func(indicator TypingIndicator)

	pendingRequests map[string]chan json.RawMessage
}

type GatewayOption func(*GatewayClient)

func WithGatewayAddr(addr string) GatewayOption {
	return func(c *GatewayClient) {
		c.addr = addr
	}
}

func WithHTTPClient(client *http.Client) GatewayOption {
	return func(c *GatewayClient) {
		c.httpClient = client
	}
}

func NewGateway(opts ...GatewayOption) (*GatewayClient, error) {
	c := &GatewayClient{
		addr:            "127.0.0.1:18789",
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		pendingRequests: make(map[string]chan json.RawMessage),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func (g *GatewayClient) Connect(ctx context.Context) error {
	url := fmt.Sprintf("ws://%s/gateway", g.addr)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to gateway: %w", err)
	}
	g.wsConn = conn

	go g.readLoop()

	return nil
}

func (g *GatewayClient) Disconnect() error {
	if g.wsConn != nil {
		return g.wsConn.Close()
	}
	return nil
}

func (g *GatewayClient) readLoop() {
	for {
		_, data, err := g.wsConn.ReadMessage()
		if err != nil {
			return
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msgType, ok := msg["type"]; ok {
			switch string(msgType) {
			case "message":
				var incoming IncomingMessage
				if rawData, ok := msg["data"]; ok {
					json.Unmarshal(rawData, &incoming)
				}
				if g.onMessage != nil {
					g.onMessage(incoming)
				}
			case "tool_call":
				var call ToolCall
				if rawData, ok := msg["data"]; ok {
					json.Unmarshal(rawData, &call)
				}
				if g.onToolCall != nil {
					g.onToolCall(call)
				}
			case "presence":
				var presence Presence
				if rawData, ok := msg["data"]; ok {
					json.Unmarshal(rawData, &presence)
				}
				if g.onPresence != nil {
					g.onPresence(presence)
				}
			case "typing":
				var indicator TypingIndicator
				if rawData, ok := msg["data"]; ok {
					json.Unmarshal(rawData, &indicator)
				}
				if g.onTyping != nil {
					g.onTyping(indicator)
				}
			}
		}
	}
}

func (g *GatewayClient) SendMessage(ctx context.Context, channelID ChannelID, msg OutgoingMessage) error {
	data, _ := json.Marshal(map[string]any{
		"type":       "message_send",
		"channel_id": channelID,
		"message":    msg,
	})
	return g.wsConn.WriteMessage(websocket.TextMessage, data)
}

func (g *GatewayClient) CreateSession(ctx context.Context, agentID AgentID, channelID ChannelID) (SessionID, error) {
	req := map[string]any{
		"agent_id":   agentID,
		"channel_id": channelID,
	}

	resp, err := g.request(ctx, "sessions_create", req)
	if err != nil {
		return "", err
	}

	var result struct {
		SessionID SessionID `json:"session_id"`
	}
	json.Unmarshal(resp, &result)
	return result.SessionID, nil
}

func (g *GatewayClient) SendToSession(ctx context.Context, sessionID SessionID, content string) error {
	req := map[string]any{
		"session_id": sessionID,
		"content":    content,
	}
	_, err := g.request(ctx, "agent_send", req)
	return err
}

func (g *GatewayClient) ListSessions(ctx context.Context) ([]Session, error) {
	resp, err := g.request(ctx, "sessions_list", nil)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	json.Unmarshal(resp, &sessions)
	return sessions, nil
}

func (g *GatewayClient) GetSessionHistory(ctx context.Context, sessionID SessionID) ([]IncomingMessage, error) {
	req := map[string]any{"session_id": sessionID}
	resp, err := g.request(ctx, "sessions_history", req)
	if err != nil {
		return nil, err
	}

	var messages []IncomingMessage
	json.Unmarshal(resp, &messages)
	return messages, nil
}

func (g *GatewayClient) InvokeTool(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	req := map[string]any{
		"name":  name,
		"input": input,
	}
	return g.request(ctx, "tools_invoke", req)
}

func (g *GatewayClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	resp, err := g.request(ctx, "tools_list", nil)
	if err != nil {
		return nil, err
	}

	var tools []ToolDefinition
	json.Unmarshal(resp, &tools)
	return tools, nil
}

func (g *GatewayClient) ListAgents(ctx context.Context) ([]Agent, error) {
	resp, err := g.request(ctx, "agents_list", nil)
	if err != nil {
		return nil, err
	}

	var agents []Agent
	json.Unmarshal(resp, &agents)
	return agents, nil
}

func (g *GatewayClient) ListChannels(ctx context.Context) ([]ChannelInfo, error) {
	resp, err := g.request(ctx, "channels_list", nil)
	if err != nil {
		return nil, err
	}

	var channels []ChannelInfo
	json.Unmarshal(resp, &channels)
	return channels, nil
}

func (g *GatewayClient) ListNodes(ctx context.Context) ([]NodeInfo, error) {
	resp, err := g.request(ctx, "nodes_list", nil)
	if err != nil {
		return nil, err
	}

	var nodes []NodeInfo
	json.Unmarshal(resp, &nodes)
	return nodes, nil
}

func (g *GatewayClient) NodeInvoke(ctx context.Context, nodeID NodeID, action string, input json.RawMessage) (json.RawMessage, error) {
	req := map[string]any{
		"node_id": nodeID,
		"action":  action,
		"input":   input,
	}
	return g.request(ctx, "node_invoke", req)
}

func (g *GatewayClient) request(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	reqID := fmt.Sprintf("%d", time.Now().UnixNano())

	req := map[string]any{
		"id":     reqID,
		"method": method,
		"params": params,
	}

	data, _ := json.Marshal(req)
	err := g.wsConn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return nil, err
	}

	ch := make(chan json.RawMessage, 1)
	g.mu.Lock()
	g.pendingRequests[reqID] = ch
	g.mu.Unlock()

	select {
	case <-ctx.Done():
		g.mu.Lock()
		delete(g.pendingRequests, reqID)
		g.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		g.mu.Lock()
		delete(g.pendingRequests, reqID)
		g.mu.Unlock()

		var result struct {
			Error  string          `json:"error,omitempty"`
			Result json.RawMessage `json:"result,omitempty"`
		}
		json.Unmarshal(resp, &result)
		if result.Error != "" {
			return nil, fmt.Errorf("gateway error: %s", result.Error)
		}
		return result.Result, nil
	}
}

func (g *GatewayClient) OnMessage(handler func(msg IncomingMessage)) {
	g.onMessage = handler
}

func (g *GatewayClient) OnToolCall(handler func(call ToolCall)) {
	g.onToolCall = handler
}

func (g *GatewayClient) OnPresence(handler func(presence Presence)) {
	g.onPresence = handler
}

func (g *GatewayClient) OnTyping(handler func(indicator TypingIndicator)) {
	g.onTyping = handler
}
