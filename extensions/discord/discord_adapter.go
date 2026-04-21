package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	BotToken   string   `json:"bot_token"`
	APIBaseURL string   `json:"api_base_url,omitempty"`
	ChannelIDs []string `json:"channel_ids,omitempty"`
	PollEvery  int      `json:"poll_every,omitempty"`
}

type DiscordExtension struct {
	config         Config
	client         *http.Client
	apiBaseURL     string
	lastMessageIDs map[string]string
	stdin          io.Reader
	stdout         io.Writer
	stderr         io.Writer
}

func NewDiscordExtension(cfg Config) *DiscordExtension {
	baseURL := strings.TrimRight(cfg.APIBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://discord.com/api/v10"
	}
	return &DiscordExtension{
		config:         cfg,
		client:         &http.Client{Timeout: 10 * time.Second},
		apiBaseURL:     baseURL,
		lastMessageIDs: map[string]string{},
		stdin:          os.Stdin,
		stdout:         os.Stdout,
		stderr:         os.Stderr,
	}
}

func (e *DiscordExtension) Run(ctx context.Context) error {
	if len(e.config.ChannelIDs) == 0 {
		return fmt.Errorf("channel_ids required")
	}

	interval := time.Duration(e.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}

	if err := e.pollOnce(ctx); err != nil {
		fmt.Fprintf(e.stderr, "discord poll error: %v\n", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := e.pollOnce(ctx); err != nil {
				fmt.Fprintf(e.stderr, "discord poll error: %v\n", err)
			}
		}
	}
}

type discordMessage struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Author  struct {
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"author"`
}

func (e *DiscordExtension) pollOnce(ctx context.Context) error {
	for _, channelID := range e.config.ChannelIDs {
		messages, err := e.fetchMessages(ctx, channelID, e.lastMessageIDs[channelID])
		if err != nil {
			return err
		}

		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i]
			e.lastMessageIDs[channelID] = message.ID

			if message.Author.Bot || strings.TrimSpace(message.Content) == "" {
				continue
			}

			payload := map[string]any{
				"action":     "message",
				"channel":    "discord",
				"chat_id":    channelID,
				"text":       message.Content,
				"user_id":    message.Author.Username,
				"message_id": message.ID,
			}
			if err := json.NewEncoder(e.stdout).Encode(payload); err != nil {
				return err
			}

			reply, err := e.readReply()
			if err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}
			if reply != "" {
				if err := e.sendMessage(channelID, reply); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (e *DiscordExtension) fetchMessages(ctx context.Context, channelID, after string) ([]discordMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.apiBaseURL+"/channels/"+channelID+"/messages?limit=50", nil)
	if err != nil {
		return nil, err
	}
	if after != "" {
		query := req.URL.Query()
		query.Set("after", after)
		req.URL.RawQuery = query.Encode()
	}
	req.Header.Set("Authorization", "Bot "+e.config.BotToken)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discord fetch failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var messages []discordMessage
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (e *DiscordExtension) readReply() (string, error) {
	var response map[string]any
	if err := json.NewDecoder(e.stdin).Decode(&response); err != nil {
		return "", err
	}
	reply, _ := response["text"].(string)
	return strings.TrimSpace(reply), nil
}

func (e *DiscordExtension) sendMessage(channelID, text string) error {
	body, _ := json.Marshal(map[string]string{"content": text})
	req, err := http.NewRequest(http.MethodPost, e.apiBaseURL+"/channels/"+channelID+"/messages", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+e.config.BotToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord send failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
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

	ext := NewDiscordExtension(cfg)
	if err := ext.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
