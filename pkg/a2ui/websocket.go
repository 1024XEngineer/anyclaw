package a2ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WSHandler struct {
	protocol  *ProtocolHandler
	clients   map[string]*WSClient
	clientsMu sync.RWMutex
	broadcast chan *WSMessage
}

type WSClient struct {
	ID        string
	SessionID string
	Conn      *websocket.Conn
	Send      chan []byte
	Handler   *WSHandler
	Closed    bool
	Mu        sync.Mutex
}

type WSMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func NewWSHandler(protocol *ProtocolHandler) *WSHandler {
	return &WSHandler{
		protocol:  protocol,
		clients:   make(map[string]*WSClient),
		broadcast: make(chan *WSMessage, 256),
	}
}

func (h *WSHandler) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &WSClient{
		ID:      generateID("ws"),
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Handler: h,
	}

	h.registerClient(client)
	defer h.unregisterClient(client)

	go client.writePump()
	go client.readPump()

	<-make(chan struct{})
}

func (h *WSHandler) registerClient(client *WSClient) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	h.clients[client.ID] = client
}

func (h *WSHandler) unregisterClient(client *WSClient) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	delete(h.clients, client.ID)
}

func (h *WSHandler) GetClient(id string) (*WSClient, bool) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	c, ok := h.clients[id]
	return c, ok
}

func (h *WSHandler) SendToSession(sessionID string, msgType string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	msg := &WSMessage{
		Type:      msgType,
		SessionID: sessionID,
		Payload:   payload,
	}

	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	for _, client := range h.clients {
		if client.SessionID == sessionID && !client.Closed {
			select {
			case client.Send <- msg.MustBytes():
			default:
			}
		}
	}

	return nil
}

func (h *WSHandler) Broadcast(msgType string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	msg := &WSMessage{
		Type:    msgType,
		Payload: payload,
	}

	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	for _, client := range h.clients {
		if !client.Closed {
			select {
			case client.Send <- msg.MustBytes():
			default:
			}
		}
	}

	return nil
}

func (c *WSClient) readPump() {
	defer func() {
		c.Conn.Close()
		c.Mu.Lock()
		c.Closed = true
		c.Mu.Unlock()
	}()

	c.Conn.SetReadLimit(512 * 1024)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError("invalid_message", err.Error())
			continue
		}

		c.handleMessage(&msg)
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) handleMessage(msg *WSMessage) {
	ctx := context.Background()

	switch msg.Type {
	case "join":
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("invalid_payload", err.Error())
			return
		}
		c.SessionID = payload.SessionID
		c.sendResponse("joined", map[string]string{"session_id": c.SessionID})

	case "chat":
		var payload struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("invalid_payload", err.Error())
			return
		}

		if c.SessionID == "" {
			c.SessionID = c.Handler.protocol.GetOrCreateSession("").ID
		}

		req := &Request{
			ID:        generateID("req"),
			Type:      MsgTypeRequest,
			Method:    "chat",
			Timestamp: time.Now().Unix(),
		}
		req.Params, _ = json.Marshal(map[string]interface{}{
			"message":    payload.Message,
			"session_id": c.SessionID,
		})

		c.Handler.protocol.SendToSession(c.SessionID, "thinking", map[string]bool{"thinking": true})

		resp, _ := c.Handler.protocol.HandleRequest(ctx, req)
		c.sendResponse("chat_response", resp.Result)

		c.Handler.protocol.SendToSession(c.SessionID, "complete", resp.Result)

	case "tool":
		var payload struct {
			Tool  string                 `json:"tool"`
			Input map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("invalid_payload", err.Error())
			return
		}

		req := &Request{
			ID:        generateID("req"),
			Type:      MsgTypeRequest,
			Method:    "tool",
			Timestamp: time.Now().Unix(),
		}
		req.Params, _ = json.Marshal(payload)

		resp, _ := c.Handler.protocol.HandleRequest(ctx, req)
		c.sendResponse("tool_response", resp.Result)

	case "canvas":
		var payload struct {
			Operation string `json:"operation"`
			Content   string `json:"content,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("invalid_payload", err.Error())
			return
		}

		req := &Request{
			ID:        generateID("req"),
			Type:      MsgTypeRequest,
			Method:    "canvas",
			Timestamp: time.Now().Unix(),
		}
		req.Params, _ = json.Marshal(map[string]interface{}{
			"session_id": c.SessionID,
			"operation":  payload.Operation,
			"content":    payload.Content,
		})

		resp, _ := c.Handler.protocol.HandleRequest(ctx, req)
		c.sendResponse("canvas_response", resp.Result)

	case "approval":
		var payload struct {
			ApprovalID string `json:"approval_id"`
			Approved   bool   `json:"approved"`
			Comment    string `json:"comment,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError("invalid_payload", err.Error())
			return
		}

		req := &Request{
			ID:        generateID("req"),
			Type:      MsgTypeRequest,
			Method:    "approval",
			Timestamp: time.Now().Unix(),
		}
		req.Params, _ = json.Marshal(payload)

		c.Handler.protocol.HandleRequest(ctx, req)

	case "ping":
		c.sendResponse("pong", map[string]int64{"timestamp": time.Now().Unix()})

	default:
		c.sendError("unknown_message_type", fmt.Sprintf("unknown type: %s", msg.Type))
	}
}

func (c *WSClient) sendResponse(msgType string, data interface{}) {
	payload, _ := json.Marshal(data)
	msg := WSMessage{
		Type:    msgType,
		Payload: payload,
	}
	c.Send <- msg.MustBytes()
}

func (c *WSClient) sendError(code, message string) {
	payload, _ := json.Marshal(map[string]string{"code": code, "message": message})
	msg := WSMessage{
		Type:    "error",
		Payload: payload,
	}
	c.Send <- msg.MustBytes()
}

func (m *WSMessage) MustBytes() []byte {
	data, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return data
}

type InMemoryCanvas struct {
	mu      sync.RWMutex
	content map[string]string
}

func NewInMemoryCanvas() *InMemoryCanvas {
	return &InMemoryCanvas{
		content: make(map[string]string),
	}
}

func (c *InMemoryCanvas) Get(sessionID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	content, ok := c.content[sessionID]
	return content, ok
}

func (c *InMemoryCanvas) Set(sessionID, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.content[sessionID] = content
}

func (c *InMemoryCanvas) Update(sessionID string, ops []CanvasOp) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	content := c.content[sessionID]
	runes := []rune(content)

	for _, op := range ops {
		switch op.Type {
		case "insert":
			if op.Start > len(runes) {
				op.Start = len(runes)
			}
			runes = append(runes[:op.Start], append([]rune(op.Content), runes[op.Start:]...)...)
		case "delete":
			if op.Start < len(runes) && op.End <= len(runes) && op.Start < op.End {
				runes = append(runes[:op.Start], runes[op.End:]...)
			}
		case "replace":
			if op.Start < len(runes) && op.End <= len(runes) {
				runes = append(runes[:op.Start], append([]rune(op.Content), runes[op.End:]...)...)
			}
		}
	}

	c.content[sessionID] = string(runes)
	return nil
}

type SimpleApprovalProvider struct {
	mu          sync.RWMutex
	approvals   map[string]*ApprovalRequest
	autoApprove bool
}

func NewSimpleApprovalProvider(autoApprove bool) *SimpleApprovalProvider {
	return &SimpleApprovalProvider{
		approvals:   make(map[string]*ApprovalRequest),
		autoApprove: autoApprove,
	}
}

func (p *SimpleApprovalProvider) Request(ctx context.Context, req *ApprovalRequest) (bool, string) {
	if p.autoApprove {
		return true, "auto-approved"
	}

	p.mu.Lock()
	p.approvals[req.ID] = req
	p.mu.Unlock()

	return false, "approval required"
}

func (p *SimpleApprovalProvider) OnApproval(ctx context.Context, id string, approved bool, comment string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.approvals[id]; !ok {
		return fmt.Errorf("approval not found")
	}

	delete(p.approvals, id)
	return nil
}
