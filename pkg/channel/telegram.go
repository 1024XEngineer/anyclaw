package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type TelegramAdapter struct {
	base        BaseAdapter
	config      config.TelegramChannelConfig
	router      *Router
	baseURL     string
	client      *http.Client
	offset      int64
	sessions    map[string]string
	appendEvent func(eventType string, sessionID string, payload map[string]any)
}

func NewTelegramAdapter(cfg config.TelegramChannelConfig, router *Router, appendEvent func(eventType string, sessionID string, payload map[string]any)) *TelegramAdapter {
	return &TelegramAdapter{
		base:        NewBaseAdapter("telegram", cfg.Enabled && cfg.BotToken != ""),
		config:      cfg,
		router:      router,
		baseURL:     "https://api.telegram.org/bot" + cfg.BotToken,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
		sessions:    make(map[string]string),
	}
}

func (a *TelegramAdapter) Name() string {
	return "telegram"
}

func (a *TelegramAdapter) Enabled() bool {
	return a.config.Enabled && a.config.BotToken != "" && (strings.TrimSpace(a.config.ChatID) != "" || true)
}

func (a *TelegramAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *TelegramAdapter) Run(ctx context.Context, runMessage InboundHandler) error {
	a.base.setRunning(true)
	defer a.base.setRunning(false)
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := a.pollOnce(ctx, runMessage); err != nil {
			a.base.setError(err)
			a.append("channel.telegram.error", "", map[string]any{"error": err.Error()})
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

func (a *TelegramAdapter) pollOnce(ctx context.Context, runMessage InboundHandler) error {
	u := fmt.Sprintf("%s/getUpdates?timeout=1&offset=%d", a.baseURL, a.offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				Text string `json:"text"`
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				From struct {
					Username string `json:"username"`
				} `json:"from"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	for _, update := range payload.Result {
		a.offset = update.UpdateID + 1
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		if strings.TrimSpace(a.config.ChatID) != "" && chatID != strings.TrimSpace(a.config.ChatID) {
			continue
		}
		text := strings.TrimSpace(update.Message.Text)
		if text == "" {
			continue
		}
		decision := a.router.Decide(RouteRequest{Channel: "telegram", Source: chatID, Text: text})
		sessionID := a.sessions[decision.Key]
		if decision.SessionID != "" {
			sessionID = decision.SessionID
		}

		sessionID, response, err := runMessage(ctx, sessionID, text, map[string]string{"channel": "telegram", "chat_id": chatID, "username": update.Message.From.Username, "reply_target": chatID, "message_id": strconv.FormatInt(update.UpdateID, 10)})
		if err != nil {
			return err
		}
		if sessionID != "" {
			a.sessions[decision.Key] = sessionID
		}
		if err := a.sendMessage(ctx, chatID, response); err != nil {
			return err
		}
		a.base.markActivity()
		a.append("channel.telegram.message", sessionID, map[string]any{
			"chat_id":   chatID,
			"text":      text,
			"route":     decision.Key,
			"agent":     decision.Agent,
			"workspace": decision.Workspace,
		})
	}
	return nil
}

func (a *TelegramAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *TelegramAdapter) sendMessage(ctx context.Context, chatID string, text string) error {
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/sendMessage", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send failed: %s", resp.Status)
	}
	return nil
}
