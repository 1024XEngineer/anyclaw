package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

type Config struct {
	ShipURL      string `json:"ship_url"`
	ShipCode     string `json:"ship_code"`
	ShipName     string `json:"ship_name"`
	PollInterval int    `json:"poll_interval"`
}

type TlonExtension struct {
	config   Config
	client   *http.Client
	baseURL  string
	shipName string
	seenMsgs map[string]bool
}

func NewTlonExtension(cfg Config) *TlonExtension {
	baseURL := strings.TrimRight(cfg.ShipURL, "/")
	jar, _ := cookiejar.New(nil)
	return &TlonExtension{
		config:   cfg,
		client:   &http.Client{Timeout: 30 * time.Second, Jar: jar},
		baseURL:  baseURL,
		shipName: cfg.ShipName,
		seenMsgs: make(map[string]bool),
	}
}

func (e *TlonExtension) Run(ctx context.Context) error {
	if err := e.login(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	interval := time.Duration(e.config.PollInterval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Fprintf(os.Stderr, "tlon adapter started, ship=%s, url=%s\n", e.shipName, e.baseURL)

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "tlon poll error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *TlonExtension) login(ctx context.Context) error {
	body := strings.NewReader("password=" + e.config.ShipCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/~/login", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	fmt.Fprintf(os.Stderr, "tlon logged in to %s\n", e.shipName)
	return nil
}

func (e *TlonExtension) pollOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/~/scry/channel.json", nil)
	if err != nil {
		return err
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("scry failed: %s: %s", resp.Status, string(body))
	}

	var channels []struct {
		ID       string `json:"id"`
		Path     string `json:"path"`
		Messages []struct {
			ID      string `json:"id"`
			Author  string `json:"author"`
			Content string `json:"content"`
			Time    int64  `json:"time"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &channels); err != nil {
		return err
	}

	for _, ch := range channels {
		for _, msg := range ch.Messages {
			if e.seenMsgs[msg.ID] {
				continue
			}
			e.seenMsgs[msg.ID] = true

			text := strings.TrimSpace(msg.Content)
			if text == "" {
				continue
			}

			input := map[string]any{
				"action":  "message",
				"channel": "tlon",
				"chat_id": ch.Path,
				"text":    text,
				"user_id": msg.Author,
			}
			data, _ := json.Marshal(input)
			fmt.Println(string(data))

			var response map[string]any
			if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			if reply, ok := response["text"].(string); ok && reply != "" {
				if err := e.sendMessage(ctx, ch.Path, reply); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (e *TlonExtension) sendMessage(ctx context.Context, chatPath, text string) error {
	payload := map[string]any{
		"action": "poke",
		"app":    "chat-hook",
		"mark":   "json",
		"body": map[string]any{
			"path":    chatPath,
			"content": text,
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/~/channel/0", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message failed: %s: %s", resp.Status, string(respBody))
	}

	fmt.Fprintf(os.Stderr, "tlon message sent to %s\n", chatPath)
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

	ext := NewTlonExtension(cfg)
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
