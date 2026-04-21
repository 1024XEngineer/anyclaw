package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Config holds the Zalo personal account configuration.
type Config struct {
	Cookie       string `json:"cookie"`
	IMEI         string `json:"imei"`
	PollInterval int    `json:"poll_interval"`
}

// ZaloExtension implements a polling-based adapter for Zalo personal accounts.
// NOTE: Zalo's internal web API is undocumented. All endpoints below are
// reverse-engineered placeholders and may break without notice.
type ZaloExtension struct {
	config  Config
	client  *http.Client
	baseURL string
	// Last seen message timestamp to avoid duplicates.
	lastTS int64
}

func NewZaloExtension(cfg Config) *ZaloExtension {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10
	}
	return &ZaloExtension{
		config: cfg,
		client: &http.Client{Timeout: 15 * time.Second},
		// Placeholder base URL — the real Zalo web API lives behind
		// https://tt-ml.zalo.me and similar internal endpoints.
		baseURL: "https://tt-ml.zalo.me/api",
	}
}

// Run starts the polling loop.
func (e *ZaloExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial poll
	if err := e.pollOnce(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "zalouser poll error: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := e.pollOnce(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "zalouser poll error: %v\n", err)
			}
		}
	}
}

// pollOnce fetches new conversations/messages from Zalo.
//
// Placeholder API flow (reverse-engineered, not officially documented):
//
//	GET /api/message/getpoll?imei=<imei>
//	Headers: Cookie: <session-cookie>
//
// Expected response (speculative):
//
//	{
//	  "data": {
//	    "conversations": [
//	      {
//	        "id": "12345",
//	        "last_message": {
//	          "text": "Hello",
//	          "sender_id": "67890",
//	          "sender_name": "Nguyen Van A",
//	          "timestamp": 1700000000
//	        }
//	      }
//	    ]
//	  }
//	}
func (e *ZaloExtension) pollOnce(ctx context.Context) error {
	// Build the poll URL with IMEI parameter.
	// NOTE: The exact query parameter names and response shape are unknown.
	u := fmt.Sprintf("%s/message/getpoll?imei=%s&t=%d",
		e.baseURL, e.config.IMEI, time.Now().UnixMilli())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}

	// Attach the session cookie for authentication.
	req.Header.Set("Cookie", e.config.Cookie)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("zalouser poll returned %s: %s", resp.Status, string(body))
	}

	var payload struct {
		Data struct {
			Conversations []struct {
				ID          string `json:"id"`
				LastMessage struct {
					Text       string `json:"text"`
					SenderID   string `json:"sender_id"`
					SenderName string `json:"sender_name"`
					Timestamp  int64  `json:"timestamp"`
				} `json:"last_message"`
			} `json:"conversations"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	for _, conv := range payload.Data.Conversations {
		msg := conv.LastMessage
		if msg.Text == "" {
			continue
		}
		// Skip messages we have already seen.
		if msg.Timestamp <= e.lastTS {
			continue
		}
		e.lastTS = msg.Timestamp

		// Emit message to AnyClaw core via stdout.
		out := map[string]any{
			"action":      "message",
			"channel":     "zalouser",
			"chat_id":     conv.ID,
			"text":        msg.Text,
			"user_id":     msg.SenderID,
			"sender_name": msg.SenderName,
		}
		data, _ := json.Marshal(out)
		fmt.Println(string(data))

		// Read reply from AnyClaw core via stdin.
		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			fmt.Fprintf(os.Stderr, "zalouser failed to read response: %v\n", err)
			continue
		}

		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.sendMessage(ctx, conv.ID, reply); err != nil {
				fmt.Fprintf(os.Stderr, "zalouser send error: %v\n", err)
			}
		}
	}

	return nil
}

// sendMessage delivers a text reply to a Zalo conversation.
//
// Placeholder API call (reverse-engineered, not officially documented):
//
//	POST /api/message/send
//	Headers: Cookie: <session-cookie>, Content-Type: application/json
//	Body: {"to_id": "<chat_id>", "message": "<text>", "imei": "<imei>"}
func (e *ZaloExtension) sendMessage(ctx context.Context, chatID string, text string) error {
	u := fmt.Sprintf("%s/message/send", e.baseURL)

	payload := map[string]string{
		"to_id":   chatID,
		"message": text,
		"imei":    e.config.IMEI,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", e.config.Cookie)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("zalouser send failed: %s: %s", resp.Status, string(respBody))
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

	ext := NewZaloExtension(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := ext.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
