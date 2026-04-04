package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type WhatsAppAdapter struct {
	base             BaseAdapter
	config           config.WhatsAppChannelConfig
	router           *Router
	client           *http.Client
	appendEvent      func(eventType string, sessionID string, payload map[string]any)
	sessions         map[string]string
	pendingResponses map[string]string
	replyTargets     map[string]string
	processed        map[string]time.Time
	mu               sync.Mutex
}

func NewWhatsAppAdapter(cfg config.WhatsAppChannelConfig, router *Router, appendEvent func(eventType string, sessionID string, payload map[string]any)) *WhatsAppAdapter {
	return &WhatsAppAdapter{
		base:             NewBaseAdapter("whatsapp", cfg.Enabled && cfg.AccessToken != "" && cfg.PhoneNumberID != ""),
		config:           cfg,
		router:           router,
		client:           &http.Client{Timeout: 20 * time.Second},
		appendEvent:      appendEvent,
		sessions:         make(map[string]string),
		pendingResponses: make(map[string]string),
		replyTargets:     make(map[string]string),
		processed:        make(map[string]time.Time),
	}
}

func (a *WhatsAppAdapter) Name() string { return "whatsapp" }

func (a *WhatsAppAdapter) Enabled() bool {
	return a.config.Enabled && strings.TrimSpace(a.config.AccessToken) != "" && strings.TrimSpace(a.config.PhoneNumberID) != ""
}

func (a *WhatsAppAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *WhatsAppAdapter) Run(ctx context.Context, handle InboundHandler) error {
	a.base.setRunning(true)
	defer a.base.setRunning(false)
	<-ctx.Done()
	return nil
}

func (a *WhatsAppAdapter) HandleInbound(ctx context.Context, source string, text string, messageID string, profileName string, handle InboundHandler) (string, string, error) {
	return a.HandleInboundWithMeta(ctx, source, text, messageID, profileName, nil, handle)
}

func (a *WhatsAppAdapter) HandleInboundWithMeta(ctx context.Context, source string, text string, messageID string, profileName string, meta map[string]string, handle InboundHandler) (string, string, error) {
	if a.seen(messageID) {
		return "", "", nil
	}
	decision := a.router.Decide(RouteRequest{Channel: "whatsapp", Source: source, Text: text})
	sessionID := a.sessions[decision.Key]
	if decision.SessionID != "" {
		sessionID = decision.SessionID
	}
	a.replyTargets[decision.Key] = source

	if meta == nil {
		meta = map[string]string{}
	}
	meta["channel"] = "whatsapp"
	meta["user_id"] = source
	meta["username"] = profileName
	meta["reply_target"] = source
	meta["message_id"] = messageID
	meta["sender"] = profileName

	sessionID, response, err := handle(ctx, sessionID, text, meta)
	if err != nil {
		return "", "", err
	}
	if sessionID != "" {
		a.sessions[decision.Key] = sessionID
	}
	if err := a.sendMessage(ctx, source, response); err != nil {
		return "", "", err
	}
	a.base.markActivity()
	a.append("channel.whatsapp.message", sessionID, map[string]any{
		"source":    source,
		"text":      text,
		"route":     decision.Key,
		"agent":     decision.Agent,
		"workspace": decision.Workspace,
	})
	return sessionID, response, nil
}

func (a *WhatsAppAdapter) HandleStatus(sessionID string, status string, messageID string, recipient string) {
	a.append("channel.whatsapp.status", sessionID, map[string]any{
		"status":     status,
		"message_id": messageID,
		"recipient":  recipient,
	})
}

func (a *WhatsAppAdapter) sendMessage(ctx context.Context, to string, text string) error {
	to = strings.TrimSpace(to)
	if to == "" {
		to = strings.TrimSpace(a.config.DefaultRecipient)
	}
	if to == "" {
		return nil
	}
	version := strings.TrimSpace(a.config.APIVersion)
	if version == "" {
		version = "v20.0"
	}
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", version, a.config.PhoneNumberID)
	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text": map[string]any{
			"body": text,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp send failed: %s", resp.Status)
	}
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if payload != nil {
		a.append("channel.whatsapp.outbound", "", payload)
	}
	return nil
}

func (a *WhatsAppAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *WhatsAppAdapter) seen(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for key, ts := range a.processed {
		if time.Since(ts) > 30*time.Minute {
			delete(a.processed, key)
		}
	}
	if _, ok := a.processed[id]; ok {
		return true
	}
	a.processed[id] = time.Now().UTC()
	return false
}
