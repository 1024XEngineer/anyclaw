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
	Homeserver  string   `json:"homeserver"`
	AccessToken string   `json:"access_token"`
	UserID      string   `json:"user_id,omitempty"`
	Rooms       []string `json:"rooms,omitempty"`
	PollEvery   int      `json:"poll_every"`
}

type MatrixExtension struct {
	config    Config
	client    *http.Client
	syncToken string
	baseURL   string
}

func NewMatrixExtension(cfg Config) *MatrixExtension {
	hs := strings.TrimRight(cfg.Homeserver, "/")
	return &MatrixExtension{
		config:  cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: hs,
	}
}

func (e *MatrixExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "matrix sync error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *MatrixExtension) pollOnce(ctx context.Context) error {
	syncURL := fmt.Sprintf("%s/_matrix/client/v3/sync?timeout=0", e.baseURL)
	if e.syncToken != "" {
		syncURL += fmt.Sprintf("&since=%s", e.syncToken)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, syncURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.AccessToken)

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
		return fmt.Errorf("sync failed: %s: %s", resp.Status, string(body))
	}

	var syncResp struct {
		NextBatch string `json:"next_batch"`
		Rooms     struct {
			Join map[string]struct {
				Timeline struct {
					Events []struct {
						Type    string `json:"type"`
						Sender  string `json:"sender"`
						Content struct {
							Body    string `json:"body"`
							MsgType string `json:"msgtype"`
						} `json:"content"`
					} `json:"events"`
				} `json:"timeline"`
			} `json:"join"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(body, &syncResp); err != nil {
		return err
	}

	e.syncToken = syncResp.NextBatch

	for roomID, room := range syncResp.Rooms.Join {
		if len(e.config.Rooms) > 0 && !e.isRoomAllowed(roomID) {
			continue
		}

		for _, event := range room.Timeline.Events {
			if event.Type != "m.room.message" {
				continue
			}
			if event.Content.MsgType != "m.text" {
				continue
			}
			if e.config.UserID != "" && event.Sender == e.config.UserID {
				continue
			}

			input := map[string]any{
				"action":  "message",
				"channel": "matrix",
				"chat_id": roomID,
				"text":    event.Content.Body,
				"user_id": event.Sender,
			}
			data, _ := json.Marshal(input)
			fmt.Println(string(data))

			var response map[string]any
			if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			if reply, ok := response["text"].(string); ok && reply != "" {
				if err := e.sendMessage(ctx, roomID, reply); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (e *MatrixExtension) isRoomAllowed(roomID string) bool {
	for _, room := range e.config.Rooms {
		if room == roomID {
			return true
		}
	}
	return false
}

func (e *MatrixExtension) sendMessage(ctx context.Context, roomID, text string) error {
	txnID := fmt.Sprintf("anyclaw.%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s", e.baseURL, roomID, txnID)

	payload := map[string]any{
		"msgtype": "m.text",
		"body":    text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.AccessToken)
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

	ext := NewMatrixExtension(cfg)
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
