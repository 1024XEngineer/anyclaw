package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebhookHandler manages incoming webhooks from external services
type WebhookHandler struct {
	mu      sync.RWMutex
	hooks   map[string]*Webhook
	history []WebhookEvent
	maxHist int
}

type Webhook struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Secret       string            `json:"secret,omitempty"`
	Agent        string            `json:"agent,omitempty"`
	Template     string            `json:"template,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"created_at"`
	LastTrigger  *time.Time        `json:"last_trigger,omitempty"`
	TriggerCount int               `json:"trigger_count"`
}

type WebhookEvent struct {
	ID        string            `json:"id"`
	WebhookID string            `json:"webhook_id"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	Response  string            `json:"response,omitempty"`
	Status    string            `json:"status"`
	Error     string            `json:"error,omitempty"`
	Duration  time.Duration     `json:"duration"`
}

func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		hooks:   make(map[string]*Webhook),
		maxHist: 100,
	}
}

func (wh *WebhookHandler) Register(webhook *Webhook) error {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	if webhook.ID == "" {
		webhook.ID = fmt.Sprintf("wh-%d", time.Now().UnixNano())
	}
	if webhook.Path == "" {
		webhook.Path = fmt.Sprintf("/webhooks/%s", webhook.ID)
	}
	webhook.CreatedAt = time.Now()
	webhook.Enabled = true

	wh.hooks[webhook.ID] = webhook
	return nil
}

func (wh *WebhookHandler) Unregister(id string) error {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	if _, ok := wh.hooks[id]; !ok {
		return fmt.Errorf("webhook not found: %s", id)
	}
	delete(wh.hooks, id)
	return nil
}

func (wh *WebhookHandler) Get(id string) (*Webhook, bool) {
	wh.mu.RLock()
	defer wh.mu.RUnlock()
	h, ok := wh.hooks[id]
	return h, ok
}

func (wh *WebhookHandler) List() []*Webhook {
	wh.mu.RLock()
	defer wh.mu.RUnlock()

	var list []*Webhook
	for _, h := range wh.hooks {
		list = append(list, h)
	}
	return list
}

func (wh *WebhookHandler) GetHistory(limit int) []WebhookEvent {
	wh.mu.RLock()
	defer wh.mu.RUnlock()

	if limit <= 0 || limit > len(wh.history) {
		limit = len(wh.history)
	}
	return wh.history[len(wh.history)-limit:]
}

func (wh *WebhookHandler) HandleRequest(ctx context.Context, r *http.Request, processFn func(ctx context.Context, webhook *Webhook, body []byte) (string, error)) (int, []byte) {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	path := r.URL.Path

	// Find matching webhook
	var matched *Webhook
	for _, h := range wh.hooks {
		if h.Enabled && strings.HasPrefix(path, h.Path) {
			matched = h
			break
		}
	}

	if matched == nil {
		return http.StatusNotFound, []byte(`{"error":"webhook not found"}`)
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusBadRequest, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error()))
	}

	// Verify signature if secret is set
	if matched.Secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			sig = r.Header.Get("X-Signature-256")
		}
		if !verifyWebhookSignature(matched.Secret, body, sig) {
			return http.StatusUnauthorized, []byte(`{"error":"invalid signature"}`)
		}
	}

	// Record event
	event := WebhookEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		WebhookID: matched.ID,
		Timestamp: time.Now(),
		Body:      string(body),
		Status:    "processing",
	}

	// Collect headers
	event.Headers = make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			event.Headers[k] = v[0]
		}
	}

	// Process webhook
	start := time.Now()
	response, err := processFn(ctx, matched, body)
	event.Duration = time.Since(start)

	if err != nil {
		event.Status = "failed"
		event.Error = err.Error()
	} else {
		event.Status = "success"
		event.Response = response
	}

	// Update webhook stats
	now := time.Now()
	matched.LastTrigger = &now
	matched.TriggerCount++

	// Store event
	wh.history = append(wh.history, event)
	if len(wh.history) > wh.maxHist {
		wh.history = wh.history[len(wh.history)-wh.maxHist:]
	}

	if err != nil {
		return http.StatusInternalServerError, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error()))
	}

	return http.StatusOK, []byte(response)
}

func verifyWebhookSignature(secret string, body []byte, signature string) bool {
	if signature == "" {
		return false
	}

	// Remove "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

// DeviceNode represents a connected device (mobile, desktop, etc.)
type DeviceNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"` // "ios", "android", "macos", "windows", "linux"
	Capabilities []string          `json:"capabilities"`
	Status       string            `json:"status"` // "online", "offline", "paired"
	ConnectedAt  time.Time         `json:"connected_at"`
	LastSeen     time.Time         `json:"last_seen"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// NodeManager manages connected device nodes
type NodeManager struct {
	mu    sync.RWMutex
	nodes map[string]*DeviceNode
}

func NewNodeManager() *NodeManager {
	return &NodeManager{
		nodes: make(map[string]*DeviceNode),
	}
}

func (nm *NodeManager) Register(node *DeviceNode) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if node.ID == "" {
		node.ID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	node.ConnectedAt = time.Now()
	node.LastSeen = time.Now()
	node.Status = "online"

	nm.nodes[node.ID] = node
	return nil
}

func (nm *NodeManager) Unregister(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if node, ok := nm.nodes[id]; ok {
		node.Status = "offline"
	}
	return nil
}

func (nm *NodeManager) Get(id string) (*DeviceNode, bool) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	n, ok := nm.nodes[id]
	return n, ok
}

func (nm *NodeManager) List() []*DeviceNode {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var list []*DeviceNode
	for _, n := range nm.nodes {
		list = append(list, n)
	}
	return list
}

func (nm *NodeManager) Invoke(ctx context.Context, nodeID string, action string, params map[string]any) (map[string]any, error) {
	nm.mu.RLock()
	node, ok := nm.nodes[nodeID]
	nm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	if node.Status != "online" {
		return nil, fmt.Errorf("node is offline: %s", nodeID)
	}

	// Update last seen
	nm.mu.Lock()
	node.LastSeen = time.Now()
	nm.mu.Unlock()

	// Handle built-in actions
	switch action {
	case "system.run":
		return map[string]any{"status": "executed", "action": action}, nil
	case "system.notify":
		return map[string]any{"status": "notified", "action": action}, nil
	case "camera.snap":
		return map[string]any{"status": "captured", "action": action}, nil
	case "location.get":
		return map[string]any{"status": "retrieved", "action": action, "lat": 0, "lng": 0}, nil
	case "screen.record":
		return map[string]any{"status": "recording", "action": action}, nil
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (nm *NodeManager) GetCapabilities(nodeID string) ([]string, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	node, ok := nm.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	return node.Capabilities, nil
}

// Health returns node manager statistics
func (nm *NodeManager) Health() map[string]any {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	online := 0
	offline := 0
	for _, n := range nm.nodes {
		if n.Status == "online" {
			online++
		} else {
			offline++
		}
	}

	return map[string]any{
		"total":   len(nm.nodes),
		"online":  online,
		"offline": offline,
	}
}
