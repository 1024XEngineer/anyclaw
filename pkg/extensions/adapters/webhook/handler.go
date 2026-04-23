package webhook

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

// Handler manages incoming webhooks from external services.
type Handler struct {
	mu      sync.RWMutex
	hooks   map[string]*Webhook
	history []Event
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

type Event struct {
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

func NewHandler() *Handler {
	return &Handler{
		hooks:   make(map[string]*Webhook),
		maxHist: 100,
	}
}

func (h *Handler) Register(webhook *Webhook) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if webhook.ID == "" {
		webhook.ID = fmt.Sprintf("wh-%d", time.Now().UnixNano())
	}
	if webhook.Path == "" {
		webhook.Path = fmt.Sprintf("/webhooks/%s", webhook.ID)
	}
	webhook.CreatedAt = time.Now()
	webhook.Enabled = true

	h.hooks[webhook.ID] = webhook
	return nil
}

func (h *Handler) Unregister(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.hooks[id]; !ok {
		return fmt.Errorf("webhook not found: %s", id)
	}
	delete(h.hooks, id)
	return nil
}

func (h *Handler) Get(id string) (*Webhook, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	hook, ok := h.hooks[id]
	return hook, ok
}

func (h *Handler) List() []*Webhook {
	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]*Webhook, 0, len(h.hooks))
	for _, hook := range h.hooks {
		list = append(list, hook)
	}
	return list
}

func (h *Handler) GetHistory(limit int) []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.history) {
		limit = len(h.history)
	}
	return h.history[len(h.history)-limit:]
}

func (h *Handler) HandleRequest(ctx context.Context, r *http.Request, processFn func(context.Context, *Webhook, []byte) (string, error)) (int, []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	path := r.URL.Path

	var matched *Webhook
	for _, hook := range h.hooks {
		if hook.Enabled && strings.HasPrefix(path, hook.Path) {
			matched = hook
			break
		}
	}

	if matched == nil {
		return http.StatusNotFound, []byte(`{"error":"webhook not found"}`)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusBadRequest, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error()))
	}

	if matched.Secret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if signature == "" {
			signature = r.Header.Get("X-Signature-256")
		}
		if !verifySignature(matched.Secret, body, signature) {
			return http.StatusUnauthorized, []byte(`{"error":"invalid signature"}`)
		}
	}

	event := Event{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		WebhookID: matched.ID,
		Timestamp: time.Now(),
		Body:      string(body),
		Status:    "processing",
		Headers:   map[string]string{},
	}
	for key, values := range r.Header {
		if len(values) > 0 {
			event.Headers[key] = values[0]
		}
	}

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

	now := time.Now()
	matched.LastTrigger = &now
	matched.TriggerCount++

	h.history = append(h.history, event)
	if len(h.history) > h.maxHist {
		h.history = h.history[len(h.history)-h.maxHist:]
	}

	if err != nil {
		return http.StatusInternalServerError, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error()))
	}

	return http.StatusOK, []byte(response)
}

func verifySignature(secret string, body []byte, signature string) bool {
	if signature == "" {
		return false
	}

	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}
