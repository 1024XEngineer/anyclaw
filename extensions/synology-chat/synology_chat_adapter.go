package main

import (
	"bytes"
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
	ServerURL    string `json:"server_url"`
	APIToken     string `json:"api_token"`
	PollInterval int    `json:"poll_interval"`
}

type SynologyChatExtension struct {
	config      Config
	client      *http.Client
	baseURL     string
	lastMessage int64
}

func NewSynologyChatExtension(cfg Config) *SynologyChatExtension {
	return &SynologyChatExtension{
		config:  cfg,
		client:  &http.Client{Timeout: 20 * time.Second},
		baseURL: strings.TrimRight(cfg.ServerURL, "/"),
	}
}

func (e *SynologyChatExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollInterval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "synology-chat poll error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *SynologyChatExtension) pollOnce(ctx context.Context) error {
	params := url.Values{}
	params.Set("token", e.config.APIToken)
	params.Set("method", "get_message")
	if e.lastMessage > 0 {
		params.Set("last_id", strconv.FormatInt(e.lastMessage, 10))
	}

	u := fmt.Sprintf("%s/entrypoint.cgi?%s", e.baseURL, params.Encode())
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
		Success bool `json:"success"`
		Data    struct {
			Messages []struct {
				ID       int64  `json:"id"`
				UserID   string `json:"user_id"`
				Username string `json:"username"`
				ChatID   string `json:"chat_id"`
				Text     string `json:"text"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.Success {
		return fmt.Errorf("synology-chat get_message returned success=false")
	}

	for _, msg := range payload.Data.Messages {
		if msg.ID <= e.lastMessage {
			continue
		}
		e.lastMessage = msg.ID

		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}

		input := map[string]any{
			"action":   "message",
			"channel":  "synology-chat",
			"chat_id":  msg.ChatID,
			"text":     text,
			"user_id":  msg.UserID,
			"username": msg.Username,
		}
		data, _ := json.Marshal(input)
		fmt.Println(string(data))

		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.sendMessage(ctx, msg.ChatID, reply); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *SynologyChatExtension) sendMessage(ctx context.Context, chatID string, text string) error {
	form := url.Values{}
	form.Set("token", e.config.APIToken)
	form.Set("method", "post_message")
	form.Set("payload", fmt.Sprintf(`{"text":%s,"chat_id":%s}`, strconv.Quote(text), strconv.Quote(chatID)))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/entrypoint.cgi", e.baseURL), bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("synology-chat post_message returned success=false")
	}
	return nil
}

func main() {
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

	ext := NewSynologyChatExtension(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-ctx.Done()
	}()

	if err := ext.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
