package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type SlackAdapter struct {
	base        BaseAdapter
	config      config.SlackChannelConfig
	router      *Router
	client      *http.Client
	appendEvent func(eventType string, sessionID string, payload map[string]any)
	sessions    map[string]string
	latestTS    string
}

func NewSlackAdapter(cfg config.SlackChannelConfig, router *Router, appendEvent func(eventType string, sessionID string, payload map[string]any)) *SlackAdapter {
	return &SlackAdapter{
		base:        NewBaseAdapter("slack", cfg.Enabled && cfg.BotToken != ""),
		config:      cfg,
		router:      router,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
		sessions:    make(map[string]string),
	}
}

func (a *SlackAdapter) Name() string {
	return "slack"
}

func (a *SlackAdapter) Enabled() bool {
	return a.config.Enabled && strings.TrimSpace(a.config.BotToken) != "" && strings.TrimSpace(a.config.DefaultChannel) != ""
}

func (a *SlackAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *SlackAdapter) Run(ctx context.Context, handle InboundHandler) error {
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
			a.append("channel.slack.error", "", map[string]any{"error": err.Error()})
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

func (a *SlackAdapter) pollOnce(ctx context.Context, handle InboundHandler) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/conversations.history?channel="+a.config.DefaultChannel+"&limit=10", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		OK       bool `json:"ok"`
		Messages []struct {
			Text  string `json:"text"`
			Ts    string `json:"ts"`
			User  string `json:"user"`
			BotID string `json:"bot_id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	for i := len(payload.Messages) - 1; i >= 0; i-- {
		msg := payload.Messages[i]
		if msg.Ts == "" || msg.Ts == a.latestTS || strings.TrimSpace(msg.Text) == "" || msg.BotID != "" {
			continue
		}
		a.latestTS = msg.Ts
		decision := a.router.Decide(RouteRequest{Channel: "slack", Source: a.config.DefaultChannel + ":" + msg.User, Text: msg.Text})
		sessionID := a.sessions[decision.Key]
		if decision.SessionID != "" {
			sessionID = decision.SessionID
		}
		sessionID, response, err := handle(ctx, sessionID, msg.Text, map[string]string{"channel": "slack", "channel_id": a.config.DefaultChannel, "user_id": msg.User, "reply_target": a.config.DefaultChannel, "message_id": msg.Ts})
		if err != nil {
			return err
		}
		if sessionID != "" {
			a.sessions[decision.Key] = sessionID
		}
		if err := a.sendMessage(ctx, response); err != nil {
			return err
		}
		a.base.markActivity()
		a.append("channel.slack.message", sessionID, map[string]any{
			"channel":   a.config.DefaultChannel,
			"user":      msg.User,
			"text":      msg.Text,
			"route":     decision.Key,
			"agent":     decision.Agent,
			"workspace": decision.Workspace,
		})
	}
	return nil
}

func (a *SlackAdapter) sendMessage(ctx context.Context, text string) error {
	body, _ := json.Marshal(map[string]any{"channel": a.config.DefaultChannel, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack send failed: %s", resp.Status)
	}
	return nil
}

func (a *SlackAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}
