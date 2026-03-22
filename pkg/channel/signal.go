package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type SignalAdapter struct {
	base        BaseAdapter
	config      config.SignalChannelConfig
	router      *Router
	client      *http.Client
	appendEvent func(eventType string, sessionID string, payload map[string]any)
	sessions    map[string]string
	latestTS    int64
	processed   map[string]time.Time
}

func NewSignalAdapter(cfg config.SignalChannelConfig, router *Router, appendEvent func(eventType string, sessionID string, payload map[string]any)) *SignalAdapter {
	return &SignalAdapter{
		base:        NewBaseAdapter("signal", cfg.Enabled && cfg.BaseURL != ""),
		config:      cfg,
		router:      router,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
		sessions:    make(map[string]string),
		processed:   make(map[string]time.Time),
	}
}

func (a *SignalAdapter) Name() string { return "signal" }

func (a *SignalAdapter) Enabled() bool {
	return a.config.Enabled && strings.TrimSpace(a.config.BaseURL) != "" && strings.TrimSpace(a.config.Number) != ""
}

func (a *SignalAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *SignalAdapter) Run(ctx context.Context, handle InboundHandler) error {
	a.base.setRunning(true)
	defer a.base.setRunning(false)
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := a.pollOnce(ctx, handle); err != nil {
			a.base.setError(err)
			a.append("channel.signal.error", "", map[string]any{"error": err.Error()})
		} else {
			a.base.setError(nil)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (a *SignalAdapter) pollOnce(ctx context.Context, handle InboundHandler) error {
	baseURL := strings.TrimRight(strings.TrimSpace(a.config.BaseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/receive/"+url.PathEscape(a.config.Number), nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(a.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.BearerToken))
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("signal receive failed: %s", resp.Status)
	}
	var payload []struct {
		Envelope struct {
			Timestamp  int64  `json:"timestamp"`
			Source     string `json:"source"`
			SourceName string `json:"sourceName"`
			GroupInfo  struct {
				GroupID string `json:"groupId"`
			} `json:"groupInfo"`
			DataMessage struct {
				Message     string `json:"message"`
				Attachments []struct {
					ContentType string `json:"contentType"`
					Filename    string `json:"filename"`
				} `json:"attachments"`
			} `json:"dataMessage"`
		} `json:"envelope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	for _, item := range payload {
		msg := strings.TrimSpace(item.Envelope.DataMessage.Message)
		messageID := fmt.Sprintf("%s:%d", item.Envelope.Source, item.Envelope.Timestamp)
		if msg == "" || item.Envelope.Timestamp <= a.latestTS || a.seen(messageID) {
			continue
		}
		a.latestTS = item.Envelope.Timestamp
		decision := a.router.Decide(RouteRequest{Channel: "signal", Source: item.Envelope.Source, Text: msg})
		sessionID := a.sessions[decision.Key]
		if decision.SessionID != "" {
			sessionID = decision.SessionID
		}
		replyTarget := item.Envelope.Source
		threadID := strings.TrimSpace(item.Envelope.GroupInfo.GroupID)
		if threadID != "" {
			replyTarget = threadID
		}
		meta := map[string]string{"channel": "signal", "user_id": item.Envelope.Source, "username": item.Envelope.SourceName, "reply_target": replyTarget, "thread_id": threadID, "message_id": messageID}
		if len(item.Envelope.DataMessage.Attachments) > 0 {
			meta["attachment_count"] = fmt.Sprintf("%d", len(item.Envelope.DataMessage.Attachments))
		}
		sessionID, response, err := handle(ctx, sessionID, msg, meta)
		if err != nil {
			return err
		}
		if sessionID != "" {
			a.sessions[decision.Key] = sessionID
		}
		if err := a.sendMessage(ctx, replyTarget, response); err != nil {
			return err
		}
		a.base.markActivity()
		a.append("channel.signal.message", sessionID, map[string]any{
			"source":      item.Envelope.Source,
			"source_name": item.Envelope.SourceName,
			"group_id":    threadID,
			"attachments": len(item.Envelope.DataMessage.Attachments),
			"text":        msg,
			"route":       decision.Key,
			"agent":       decision.Agent,
			"workspace":   decision.Workspace,
		})
	}
	return nil
}

func (a *SignalAdapter) sendMessage(ctx context.Context, recipient string, text string) error {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		recipient = strings.TrimSpace(a.config.DefaultRecipient)
	}
	if recipient == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"message":    text,
		"number":     a.config.Number,
		"recipients": []string{recipient},
	})
	baseURL := strings.TrimRight(strings.TrimSpace(a.config.BaseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v2/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(a.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.BearerToken))
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("signal send failed: %s", resp.Status)
	}
	return nil
}

func (a *SignalAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *SignalAdapter) seen(id string) bool {
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
