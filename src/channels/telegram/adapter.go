package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/config"
)

type Adapter struct {
	base     channel.BaseAdapter
	config   config.TelegramChannelConfig
	client   *http.Client
	baseURL  string
	offset   int64
	sessions map[string]string
}

func NewAdapter(cfg config.TelegramChannelConfig) *Adapter {
	return &Adapter{
		base:     channel.NewBaseAdapter("telegram", cfg.Enabled && cfg.BotToken != ""),
		config:   cfg,
		client:   &http.Client{Timeout: 20 * time.Second},
		baseURL:  "https://api.telegram.org/bot" + cfg.BotToken,
		sessions: make(map[string]string),
	}
}

func (a *Adapter) Name() string {
	return "telegram"
}

func (a *Adapter) Enabled() bool {
	return a.config.Enabled && a.config.BotToken != ""
}

func (a *Adapter) Run(ctx context.Context, handler channel.InboundHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		if err := a.pollUpdates(ctx, handler); err != nil {
			a.base.SetError(err)
		} else {
			a.base.SetError(nil)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (a *Adapter) pollUpdates(ctx context.Context, handler channel.InboundHandler) error {
	url := fmt.Sprintf("%s/getUpdates?timeout=1&offset=%d", a.baseURL, a.offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
					ID       int64  `json:"id"`
				} `json:"from"`
				MessageID int64 `json:"message_id"`
			} `json:"message"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	if !payload.OK {
		return fmt.Errorf("telegram API error")
	}

	for _, update := range payload.Result {
		a.offset = update.UpdateID + 1

		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		text := strings.TrimSpace(update.Message.Text)

		if text == "" {
			continue
		}

		if strings.TrimSpace(a.config.ChatID) != "" && chatID != strings.TrimSpace(a.config.ChatID) {
			continue
		}

		meta := map[string]string{
			"channel":    "telegram",
			"chat_id":    chatID,
			"username":   update.Message.From.Username,
			"user_id":    strconv.FormatInt(update.Message.From.ID, 10),
			"message_id": strconv.FormatInt(update.Message.MessageID, 10),
		}

		sessionID, response, err := handler(ctx, "", text, meta)
		if err != nil {
			continue
		}

		if sessionID != "" {
			a.sessions[chatID] = sessionID
		}

		if response != "" {
			a.sendMessage(ctx, chatID, response)
		}

		a.base.MarkActivity()
	}

	return nil
}

func (a *Adapter) sendMessage(ctx context.Context, chatID, text string) error {
	values := map[string]string{
		"chat_id": chatID,
		"text":    text,
	}

	body, _ := json.Marshal(values)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/sendMessage", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("send failed: %s", resp.Status)
	}

	return nil
}

func (a *Adapter) Status() channel.Status {
	return a.base.Status()
}
