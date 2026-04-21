package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken  string `json:"bot_token"`
	ChatID    string `json:"chat_id,omitempty"`
	PollEvery int    `json:"poll_every"`
}

type TelegramExtension struct {
	config  Config
	client  *http.Client
	offset  int64
	baseURL string
}

func NewTelegramExtension(cfg Config) *TelegramExtension {
	return &TelegramExtension{
		config:  cfg,
		client:  &http.Client{Timeout: 20 * time.Second},
		baseURL: "https://api.telegram.org/bot" + cfg.BotToken,
	}
}

func (e *TelegramExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "telegram poll error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *TelegramExtension) pollOnce(ctx context.Context) error {
	u := fmt.Sprintf("%s/getUpdates?timeout=1&offset=%d", e.baseURL, e.offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
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
		e.offset = update.UpdateID + 1
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		if strings.TrimSpace(e.config.ChatID) != "" && chatID != strings.TrimSpace(e.config.ChatID) {
			continue
		}
		text := strings.TrimSpace(update.Message.Text)
		if text == "" {
			continue
		}

		// Send message to AnyClaw core via stdin/stdout JSON protocol
		input := map[string]any{
			"action":    "message",
			"channel":   "telegram",
			"chat_id":   chatID,
			"text":      text,
			"username":  update.Message.From.Username,
			"update_id": update.UpdateID,
		}
		data, _ := json.Marshal(input)
		fmt.Println(string(data))

		// Read response from stdin
		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.sendMessage(ctx, chatID, reply); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *TelegramExtension) sendMessage(ctx context.Context, chatID string, text string) error {
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/sendMessage", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send failed: %s", resp.Status)
	}
	return nil
}

func main() {
	// Read config from environment
	configJSON := os.Getenv("ANYCLAW_EXTENSION_CONFIG")
	if configJSON == "" {
		fmt.Fprintln(os.Stderr, "missing ANYCLAW_EXTENSION_CONFIG")
		os.Exit(1)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	ext := NewTelegramExtension(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	go func() {
		// Simple signal handling could be added here
		<-ctx.Done()
	}()

	if err := ext.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
