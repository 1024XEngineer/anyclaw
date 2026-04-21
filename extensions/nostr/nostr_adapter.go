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
	RelayURL     string `json:"relay_url"`
	PrivateKey   string `json:"private_key"`
	PollInterval int    `json:"poll_interval"`
}

type NostrExtension struct {
	config  Config
	client  *http.Client
	pubkey  string
	seenIDs map[string]bool
	baseURL string
}

func NewNostrExtension(cfg Config) *NostrExtension {
	relayURL := strings.TrimRight(cfg.RelayURL, "/")
	pubkey := derivePubkey(cfg.PrivateKey)
	return &NostrExtension{
		config:  cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		pubkey:  pubkey,
		seenIDs: make(map[string]bool),
		baseURL: relayURL,
	}
}

func derivePubkey(privateKey string) string {
	return privateKey + "_pub"
}

func (e *NostrExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollInterval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Fprintf(os.Stderr, "nostr adapter started, pubkey=%s, relay=%s\n", e.pubkey, e.baseURL)

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "nostr poll error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *NostrExtension) pollOnce(ctx context.Context) error {
	queryURL := fmt.Sprintf("%s/events?author=%s&kinds=1&limit=20", e.baseURL, e.pubkey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
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
		return fmt.Errorf("query failed: %s: %s", resp.Status, string(body))
	}

	var events []struct {
		ID        string  `json:"id"`
		Pubkey    string  `json:"pubkey"`
		Kind      int     `json:"kind"`
		Content   string  `json:"content"`
		CreatedAt int64   `json:"created_at"`
		Tags      [][]any `json:"tags"`
	}
	if err := json.Unmarshal(body, &events); err != nil {
		return err
	}

	for _, event := range events {
		if event.Kind != 1 {
			continue
		}
		if event.Pubkey == e.pubkey {
			continue
		}
		if e.seenIDs[event.ID] {
			continue
		}
		e.seenIDs[event.ID] = true

		text := strings.TrimSpace(event.Content)
		if text == "" {
			continue
		}

		input := map[string]any{
			"action":  "message",
			"channel": "nostr",
			"chat_id": e.pubkey,
			"text":    text,
			"user_id": event.Pubkey,
		}
		data, _ := json.Marshal(input)
		fmt.Println(string(data))

		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.sendNote(ctx, event.Pubkey, reply); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *NostrExtension) sendNote(ctx context.Context, targetPubkey, text string) error {
	note := map[string]any{
		"pubkey":     e.pubkey,
		"kind":       1,
		"content":    text,
		"created_at": time.Now().Unix(),
		"tags":       [][]string{{"p", targetPubkey}},
	}
	body, _ := json.Marshal(note)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/events", strings.NewReader(string(body)))
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
		return fmt.Errorf("publish failed: %s: %s", resp.Status, string(respBody))
	}

	fmt.Fprintf(os.Stderr, "nostr note published to %s\n", targetPubkey)
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

	ext := NewNostrExtension(cfg)
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
