package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// GatewayConfig Gateway 配置
type GatewayConfig struct {
	Host           string        `json:"host"`
	Port           int           `json:"port"`
	Bind           string        `json:"bind"`
	ReadTimeout    time.Duration `json:"read_timeout"`
	WriteTimeout   time.Duration `json:"write_timeout"`
	MaxConnections int           `json:"max_connections"`
	AllowedOrigins []string      `json:"allowed_origins"`
	EnableTLS      bool          `json:"enable_tls"`
	CertFile       string        `json:"cert_file"`
	KeyFile        string        `json:"key_file"`
}

// Gateway Gateway 控制面
type Gateway struct {
	config     GatewayConfig
	server     *http.Server
	upgrader   websocket.Upgrader
	clients    map[string]*Client
	clientsMu  sync.RWMutex
	handlers   map[string]MessageHandler
	handlersMu sync.RWMutex
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
	startedAt  time.Time
}

// Client WebSocket 客户端
type Client struct {
	ID         string
	Conn       *websocket.Conn
	Send       chan []byte
	UserID     string
	SessionID  string
	Connected  time.Time
	LastActive time.Time
	closeOnce  sync.Once
}

// MessageHandler 消息处理器
type MessageHandler func(ctx context.Context, client *Client, message *Message) error

// Message WebSocket 消息
type Message struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    interface{}     `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// Error 错误信息
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewGateway 创建新的 Gateway
func NewGateway(config GatewayConfig) *Gateway {
	return &Gateway{
		config: config,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				if len(config.AllowedOrigins) == 0 {
					return true
				}
				origin := r.Header.Get("Origin")
				for _, allowed := range config.AllowedOrigins {
					if allowed == "*" || allowed == origin {
						return true
					}
				}
				return false
			},
		},
		clients:  make(map[string]*Client),
		handlers: make(map[string]MessageHandler),
		stopCh:   make(chan struct{}),
	}
}

// Start 启动 Gateway
func (g *Gateway) Start(ctx context.Context) error {
	g.startedAt = time.Now()
	mux := http.NewServeMux()

	// WebSocket 端点
	mux.HandleFunc("/ws", g.handleWebSocket)

	// HTTP API 端点
	mux.HandleFunc("/api/health", g.handleHealth)
	mux.HandleFunc("/api/status", g.handleStatus)
	mux.HandleFunc("/api/channels", g.handleChannels)
	mux.HandleFunc("/api/sessions", g.handleSessions)
	mux.HandleFunc("/api/tools", g.handleTools)

	// 静态文件服务
	mux.Handle("/", http.FileServer(http.Dir("web")))

	g.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", g.config.Host, g.config.Port),
		Handler:      mux,
		ReadTimeout:  g.config.ReadTimeout,
		WriteTimeout: g.config.WriteTimeout,
	}

	// 启动清理协程
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.cleanupClients(ctx)
	}()

	// 启动服务器
	if g.config.EnableTLS {
		return g.server.ListenAndServeTLS(g.config.CertFile, g.config.KeyFile)
	}
	return g.server.ListenAndServe()
}

// Stop 停止 Gateway
func (g *Gateway) Stop(ctx context.Context) error {
	g.stopOnce.Do(func() {
		close(g.stopCh)
	})

	// 关闭所有客户端连接
	g.clientsMu.Lock()
	for _, client := range g.clients {
		close(client.Send)
		client.Conn.Close()
	}
	g.clientsMu.Unlock()

	// 等待所有 goroutine 结束
	g.wg.Wait()

	// 关闭服务器
	if g.server != nil {
		return g.server.Shutdown(ctx)
	}

	return nil
}

// RegisterHandler 注册消息处理器
func (g *Gateway) RegisterHandler(method string, handler MessageHandler) {
	g.handlersMu.Lock()
	defer g.handlersMu.Unlock()

	g.handlers[method] = handler
}

// handleWebSocket 处理 WebSocket 连接
func (g *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("websocket upgrade error: %v\n", err)
		return
	}

	// 检查连接数限制
	g.clientsMu.RLock()
	if g.config.MaxConnections > 0 && len(g.clients) >= g.config.MaxConnections {
		g.clientsMu.RUnlock()
		conn.Close()
		return
	}
	g.clientsMu.RUnlock()

	// 创建客户端
	client := &Client{
		ID:         generateClientID(),
		Conn:       conn,
		Send:       make(chan []byte, 256),
		Connected:  time.Now(),
		LastActive: time.Now(),
	}

	// 注册客户端
	g.clientsMu.Lock()
	g.clients[client.ID] = client
	g.clientsMu.Unlock()

	// 启动读写协程
	g.wg.Add(2)
	go func() {
		defer g.wg.Done()
		g.readPump(client, r.Context())
	}()
	go func() {
		defer g.wg.Done()
		g.writePump(client)
	}()
}

// readPump 读取消息
func (g *Gateway) readPump(client *Client, ctx context.Context) {
	defer func() {
		g.clientsMu.Lock()
		delete(g.clients, client.ID)
		g.clientsMu.Unlock()
		client.closeOnce.Do(func() {
			client.Conn.Close()
		})
	}()

	client.Conn.SetReadLimit(4096)
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		client.LastActive = time.Now()
		return nil
	})

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				fmt.Printf("websocket read error: %v\n", err)
			}
			break
		}

		client.LastActive = time.Now()

		// 处理消息
		go g.handleMessage(ctx, client, message)
	}
}

// writePump 写入消息
func (g *Gateway) writePump(client *Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		client.closeOnce.Do(func() {
			client.Conn.Close()
		})
	}()

	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage 处理消息
func (g *Gateway) handleMessage(ctx context.Context, client *Client, data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		g.sendError(client, msg.ID, 400, "invalid message format")
		return
	}

	msg.Timestamp = time.Now().Unix()

	// 查找处理器
	g.handlersMu.RLock()
	handler, exists := g.handlers[msg.Method]
	g.handlersMu.RUnlock()

	if !exists {
		g.sendError(client, msg.ID, 404, fmt.Sprintf("method not found: %s", msg.Method))
		return
	}

	// 执行处理器
	if err := handler(ctx, client, &msg); err != nil {
		g.sendError(client, msg.ID, 500, err.Error())
		return
	}
}

// sendError 发送错误
func (g *Gateway) sendError(client *Client, id string, code int, message string) {
	response := Message{
		Type: "error",
		ID:   id,
		Error: &Error{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(response)
	if err != nil {
		return
	}

	select {
	case client.Send <- data:
	default:
		// skip when buffer is full to avoid closing channel concurrently
	}
}

// SendToClient 发送消息到客户端
func (g *Gateway) SendToClient(clientID string, message *Message) error {
	g.clientsMu.RLock()
	client, exists := g.clients[clientID]
	g.clientsMu.RUnlock()

	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	select {
	case client.Send <- data:
		return nil
	default:
		return fmt.Errorf("client send buffer full")
	}
}

// Broadcast 广播消息
func (g *Gateway) Broadcast(message *Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	g.clientsMu.RLock()
	clients := make([]*Client, 0, len(g.clients))
	for _, client := range g.clients {
		clients = append(clients, client)
	}
	g.clientsMu.RUnlock()

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
			// 跳过缓冲区已满的客户端
		}
	}

	return nil
}

// GetClients 获取所有客户端
func (g *Gateway) GetClients() []*Client {
	g.clientsMu.RLock()
	defer g.clientsMu.RUnlock()

	clients := make([]*Client, 0, len(g.clients))
	for _, client := range g.clients {
		clients = append(clients, client)
	}

	return clients
}

// GetClient 获取客户端
func (g *Gateway) GetClient(id string) (*Client, error) {
	g.clientsMu.RLock()
	defer g.clientsMu.RUnlock()

	client, exists := g.clients[id]
	if !exists {
		return nil, fmt.Errorf("client not found: %s", id)
	}

	return client, nil
}

// cleanupClients 清理不活跃的客户端
func (g *Gateway) cleanupClients(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.clientsMu.Lock()
			now := time.Now()
			for id, client := range g.clients {
				if now.Sub(client.LastActive) > 5*time.Minute {
					func() {
						defer func() {
							recover()
						}()
						close(client.Send)
					}()
					client.Conn.Close()
					delete(g.clients, id)
				}
			}
			g.clientsMu.Unlock()
		}
	}
}

// handleHealth 健康检查
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

// handleStatus 状态查询
func (g *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	g.clientsMu.RLock()
	clientCount := len(g.clients)
	g.clientsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "running",
		"clients":    clientCount,
		"uptime":     time.Since(g.startedAt).String(),
		"max_conns":  g.config.MaxConnections,
		"enable_tls": g.config.EnableTLS,
	})
}

// handleChannels 渠道列表
func (g *Gateway) handleChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

// handleSessions 会话列表
func (g *Gateway) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

// handleTools 工具列表
func (g *Gateway) handleTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

// generateClientID 生成客户端 ID
func generateClientID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("client_%d", time.Now().UnixNano())
	}
	return "client_" + hex.EncodeToString(b)
}
