package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/gorilla/websocket"
)

type pendingRequest struct {
	ch chan openClawWSFrame
}

type WSClient struct {
	url       string
	token     string
	conn      *websocket.Conn
	connected bool
	mu        sync.RWMutex
	nonce     string
	onEvent   func(event *Event)
	pending   map[string]*pendingRequest
	pendingMu sync.Mutex
	closed    chan struct{}
	closeOnce sync.Once
	writeMu   sync.Mutex
}

func NewWSClient(url, token string) *WSClient {
	return &WSClient{
		url:     url,
		token:   token,
		pending: make(map[string]*pendingRequest),
		closed:  make(chan struct{}),
	}
}

func (c *WSClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	header := http.Header{}
	if c.token != "" {
		header.Set("Authorization", "Bearer "+c.token)
	}

	dialer := websocket.DefaultDialer
	dialer.Proxy = func(req *http.Request) (*url.URL, error) {
		return nil, nil
	}
	conn, _, err := dialer.DialContext(ctx, c.url, header)
	if err != nil {
		return fmt.Errorf("failed to connect to gateway: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.nonce = uniqueID("client")
	c.mu.Unlock()

	if err := c.sendConnect(ctx); err != nil {
		conn.Close()
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	go c.readLoop()

	return nil
}

func (c *WSClient) sendConnect(ctx context.Context) error {
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	var challengeFrame openClawWSFrame
	if err := c.conn.ReadJSON(&challengeFrame); err != nil {
		return fmt.Errorf("failed to read challenge: %w", err)
	}

	if challengeFrame.Type != "event" || challengeFrame.Event != "connect.challenge" {
		return fmt.Errorf("unexpected frame type: expected connect.challenge event")
	}

	data, ok := challengeFrame.Data.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid challenge data format")
	}

	gatewayNonce, _ := data["nonce"].(string)
	if gatewayNonce == "" {
		return fmt.Errorf("nonce not provided in challenge")
	}

	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "connect",
		Params: map[string]any{
			"challenge": gatewayNonce,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := c.conn.WriteJSON(frame)
	if err != nil {
		return err
	}

	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var resp openClawWSFrame
	if err := c.conn.ReadJSON(&resp); err != nil {
		return err
	}

	if !resp.OK {
		return fmt.Errorf("connect failed: %s", resp.Error)
	}

	return nil
}

func (c *WSClient) readLoop() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.connected = false
		c.mu.Unlock()

		c.pendingMu.Lock()
		for id, p := range c.pending {
			select {
			case p.ch <- openClawWSFrame{}:
			default:
			}
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	}()

	c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	for {
		var frame openClawWSFrame
		err := c.conn.ReadJSON(&frame)
		if err != nil {
			return
		}

		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		if frame.Type == "res" && frame.ID != "" {
			c.pendingMu.Lock()
			if p, ok := c.pending[frame.ID]; ok {
				p.ch <- frame
				delete(c.pending, frame.ID)
			}
			c.pendingMu.Unlock()
			continue
		}

		if frame.Type == "event" && frame.Event != "" {
			c.mu.RLock()
			handler := c.onEvent
			c.mu.RUnlock()
			if handler != nil {
				var payload map[string]any
				if frame.Data != nil {
					if p, ok := frame.Data.(map[string]any); ok {
						payload = p
					}
				}
				event := &Event{
					Type:      frame.Event,
					SessionID: "",
					Payload:   payload,
				}
				handler(event)
			}
		}
	}
}

func (c *WSClient) call(ctx context.Context, frame openClawWSFrame) (openClawWSFrame, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return openClawWSFrame{}, fmt.Errorf("not connected to gateway")
	}
	conn := c.conn
	c.mu.RUnlock()

	ch := make(chan openClawWSFrame, 1)

	c.pendingMu.Lock()
	c.pending[frame.ID] = &pendingRequest{ch: ch}
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	err := conn.WriteJSON(frame)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, frame.ID)
		c.pendingMu.Unlock()
		return openClawWSFrame{}, err
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return openClawWSFrame{}, fmt.Errorf("connection closed")
		}
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, frame.ID)
		c.pendingMu.Unlock()
		return openClawWSFrame{}, ctx.Err()
	}
}

func (c *WSClient) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *WSClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.connected = false
		return err
	}
	return nil
}

func (c *WSClient) SendMessage(ctx context.Context, message string) (string, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "chat.send",
		Params: map[string]any{
			"message": message,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return "", err
	}

	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	response, _ := data["response"].(string)
	if response == "" {
		response, _ = data["message"].(string)
	}
	return response, nil
}

func (c *WSClient) GetStatus(ctx context.Context) (Status, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "status.get",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return Status{}, err
	}

	if !resp.OK {
		return Status{}, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return Status{}, fmt.Errorf("invalid response format")
	}

	jsonData, _ := json.Marshal(data)
	var status Status
	json.Unmarshal(jsonData, &status)
	return status, nil
}

func (c *WSClient) ListSessions(ctx context.Context) ([]Session, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "sessions.list",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var sessions []Session
	if data, ok := resp.Data.(map[string]any); ok {
		if sessionsData, ok := data["sessions"].([]any); ok {
			for _, s := range sessionsData {
				if sessionMap, ok := s.(map[string]any); ok {
					session := Session{}
					if id, ok := sessionMap["id"].(string); ok {
						session.ID = id
					}
					if title, ok := sessionMap["title"].(string); ok {
						session.Title = title
					}
					sessions = append(sessions, session)
				}
			}
		}
	}

	return sessions, nil
}

func (c *WSClient) SubscribeEvents(ctx context.Context, eventType string, handler func(*Event)) error {
	c.mu.Lock()
	c.onEvent = handler
	c.mu.Unlock()

	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "events.subscribe",
		Params: map[string]any{
			"event": eventType,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, frame)
	return err
}

func (c *WSClient) InvokeTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "tools.invoke",
		Params: map[string]any{
			"tool": toolName,
			"args": args,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return resp.Data, nil
}

func (c *WSClient) SendChatMessage(ctx context.Context, sessionID, message string) (string, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "sessions.send",
		Params: map[string]any{
			"session_id": sessionID,
			"message":    message,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return "", err
	}

	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	response, _ := data["response"].(string)
	return response, nil
}

func (c *WSClient) GetConfig(ctx context.Context) (map[string]any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "config.get",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	return data, nil
}

func (c *WSClient) SetConfig(ctx context.Context, key string, value any) error {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "config.set",
		Params: map[string]any{
			"key":   key,
			"value": value,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, frame)
	return err
}

func (c *WSClient) ListAgents(ctx context.Context) ([]string, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "agents.list",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var agents []string
	if data, ok := resp.Data.(map[string]any); ok {
		if agentsData, ok := data["agents"].([]any); ok {
			for _, a := range agentsData {
				if name, ok := a.(string); ok {
					agents = append(agents, name)
				} else if agentMap, ok := a.(map[string]any); ok {
					if name, ok := agentMap["name"].(string); ok {
						agents = append(agents, name)
					}
				}
			}
		}
	}

	return agents, nil
}

func (c *WSClient) ListChannels(ctx context.Context) ([]string, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "channels.list",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var channels []string
	if data, ok := resp.Data.(map[string]any); ok {
		if channelsData, ok := data["channels"].([]any); ok {
			for _, ch := range channelsData {
				if name, ok := ch.(string); ok {
					channels = append(channels, name)
				} else if chMap, ok := ch.(map[string]any); ok {
					if name, ok := chMap["name"].(string); ok {
						channels = append(channels, name)
					}
				}
			}
		}
	}

	return channels, nil
}

func (c *WSClient) ListTools(ctx context.Context) ([]string, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "tools.list",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var tools []string
	if data, ok := resp.Data.(map[string]any); ok {
		if toolsData, ok := data["tools"].([]any); ok {
			for _, t := range toolsData {
				if name, ok := t.(string); ok {
					tools = append(tools, name)
				} else if toolMap, ok := t.(map[string]any); ok {
					if name, ok := toolMap["name"].(string); ok {
						tools = append(tools, name)
					}
				}
			}
		}
	}

	return tools, nil
}

func (c *WSClient) AbortChat(ctx context.Context) error {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "chat.abort",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, frame)
	return err
}

func (c *WSClient) GetChatHistory(ctx context.Context, sessionID string) ([]map[string]any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "chat.history",
		Params: map[string]any{
			"session_id": sessionID,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var history []map[string]any
	if data, ok := resp.Data.(map[string]any); ok {
		if historyData, ok := data["history"].([]any); ok {
			for _, h := range historyData {
				if item, ok := h.(map[string]any); ok {
					history = append(history, item)
				}
			}
		}
	}

	return history, nil
}

func (c *WSClient) Ping(ctx context.Context) error {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "ping",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, frame)
	return err
}

func (c *WSClient) ListMethods(ctx context.Context) ([]string, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "methods.list",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var methods []string
	if data, ok := resp.Data.(map[string]any); ok {
		if methodsData, ok := data["methods"].([]any); ok {
			for _, m := range methodsData {
				if method, ok := m.(string); ok {
					methods = append(methods, method)
				}
			}
		}
	}

	return methods, nil
}

func GatewayURLFromConfig(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Gateway.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Gateway.Port
	if port == 0 {
		port = 18789
	}
	return fmt.Sprintf("ws://%s:%d/ws", host, port)
}

type PairingCodeResult struct {
	Code    string `json:"code"`
	Expires string `json:"expires"`
	Device  string `json:"device"`
	Type    string `json:"type"`
}

func (c *WSClient) GeneratePairingCode(ctx context.Context, deviceName, deviceType string) (*PairingCodeResult, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.generate",
		Params: map[string]any{
			"device_name": deviceName,
			"device_type": deviceType,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	result := &PairingCodeResult{}
	if code, ok := data["code"].(string); ok {
		result.Code = code
	}
	if expires, ok := data["expires"].(string); ok {
		result.Expires = expires
	}
	if device, ok := data["device"].(string); ok {
		result.Device = device
	}
	if deviceType, ok := data["type"].(string); ok {
		result.Type = deviceType
	}

	return result, nil
}

func (c *WSClient) ValidatePairingCode(ctx context.Context, code string) (bool, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.validate",
		Params: map[string]any{
			"code": code,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return false, err
	}

	if !resp.OK {
		return false, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return false, fmt.Errorf("invalid response format")
	}

	valid, _ := data["valid"].(bool)
	return valid, nil
}

func (c *WSClient) CompletePairing(ctx context.Context, code, deviceID, deviceName string) (map[string]any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.pair",
		Params: map[string]any{
			"code":        code,
			"device_id":   deviceID,
			"device_name": deviceName,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	return data, nil
}

func (c *WSClient) ListPairedDevices(ctx context.Context) ([]map[string]any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.list",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	var devices []map[string]any
	if data, ok := resp.Data.(map[string]any); ok {
		if devicesData, ok := data["devices"].([]any); ok {
			for _, d := range devicesData {
				if device, ok := d.(map[string]any); ok {
					devices = append(devices, device)
				}
			}
		}
	}

	return devices, nil
}

func (c *WSClient) UnpairDevice(ctx context.Context, deviceID string) error {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.unpair",
		Params: map[string]any{
			"device_id": deviceID,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.call(ctx, frame)
	return err
}

func (c *WSClient) GetPairingStatus(ctx context.Context) (map[string]any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.status",
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	return data, nil
}

func (c *WSClient) RenewPairing(ctx context.Context, deviceID string) (map[string]any, error) {
	frame := openClawWSFrame{
		Type:   "req",
		ID:     uniqueID("req"),
		Method: "device.pairing.renew",
		Params: map[string]any{
			"device_id": deviceID,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.call(ctx, frame)
	if err != nil {
		return nil, err
	}

	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	return data, nil
}
