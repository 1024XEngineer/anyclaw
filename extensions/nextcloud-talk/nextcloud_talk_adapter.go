package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	ServerURL    string `json:"server_url"`
	Username     string `json:"username"`
	AppPassword  string `json:"app_password"`
	PollInterval int    `json:"poll_interval"`
}

type NextcloudTalkExtension struct {
	config     Config
	client     *http.Client
	baseURL    string
	authHeader string
	lastMsgID  map[string]int
}

func NewNextcloudTalkExtension(cfg Config) *NextcloudTalkExtension {
	server := strings.TrimRight(cfg.ServerURL, "/")
	creds := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.AppPassword))
	return &NextcloudTalkExtension{
		config:     cfg,
		client:     &http.Client{Timeout: 30 * time.Second},
		baseURL:    server,
		authHeader: "Basic " + creds,
		lastMsgID:  make(map[string]int),
	}
}

func (e *NextcloudTalkExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollInterval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "nextcloud-talk poll error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *NextcloudTalkExtension) pollOnce(ctx context.Context) error {
	conversations, err := e.getConversations(ctx)
	if err != nil {
		return fmt.Errorf("get conversations: %w", err)
	}

	for _, conv := range conversations {
		token := conv.Token
		if token == "" {
			continue
		}

		messages, err := e.getMessages(ctx, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "nextcloud-talk get messages error for %s: %v\n", token, err)
			continue
		}

		for _, msg := range messages {
			if msg.ActorID == e.config.Username {
				continue
			}
			if msg.MessageType != "" && msg.MessageType != "comment" {
				continue
			}
			if msg.SystemMessage != "" {
				continue
			}

			lastID, seen := e.lastMsgID[token]
			if seen && msg.ID <= lastID {
				continue
			}

			e.lastMsgID[token] = msg.ID

			input := map[string]any{
				"action":     "message",
				"channel":    "nextcloud-talk",
				"chat_id":    token,
				"text":       e.parseMessage(msg.Message),
				"user_id":    msg.ActorID,
				"actor_name": msg.ActorDisplayName,
				"message_id": msg.ID,
			}
			data, _ := json.Marshal(input)
			fmt.Println(string(data))

			var response map[string]any
			if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			if reply, ok := response["text"].(string); ok && reply != "" {
				if err := e.sendMessage(ctx, token, reply); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (e *NextcloudTalkExtension) getConversations(ctx context.Context) ([]Conversation, error) {
	url := fmt.Sprintf("%s/ocs/v2.php/apps/spreed/api/v4/room", e.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", e.authHeader)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get conversations failed: %s: %s", resp.Status, string(body))
	}

	var ocsResp struct {
		OCS struct {
			Data []Conversation `json:"data"`
		} `json:"ocs"`
	}
	if err := json.Unmarshal(body, &ocsResp); err != nil {
		return nil, err
	}

	return ocsResp.OCS.Data, nil
}

func (e *NextcloudTalkExtension) getMessages(ctx context.Context, token string) ([]Message, error) {
	url := fmt.Sprintf("%s/ocs/v2.php/apps/spreed/api/v4/chat/%s", e.baseURL, token)
	if lastID, ok := e.lastMsgID[token]; ok {
		url += fmt.Sprintf("?lastKnownMessageId=%d", lastID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", e.authHeader)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get messages failed: %s: %s", resp.Status, string(body))
	}

	var ocsResp struct {
		OCS struct {
			Data []Message `json:"data"`
		} `json:"ocs"`
	}
	if err := json.Unmarshal(body, &ocsResp); err != nil {
		return nil, err
	}

	return ocsResp.OCS.Data, nil
}

func (e *NextcloudTalkExtension) sendMessage(ctx context.Context, token, text string) error {
	url := fmt.Sprintf("%s/ocs/v2.php/apps/spreed/api/v1/chat/%s", e.baseURL, token)

	payload := map[string]string{
		"message": text,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", e.authHeader)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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

func (e *NextcloudTalkExtension) parseMessage(raw string) string {
	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed.Message != "" {
		return parsed.Message
	}
	return raw
}

type Conversation struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	LastMessage struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
	} `json:"lastMessage"`
	UnreadMessages int `json:"unreadMessages"`
}

type Message struct {
	ID               int    `json:"id"`
	MessageType      string `json:"messageType"`
	SystemMessage    string `json:"systemMessage"`
	Message          string `json:"message"`
	ActorID          string `json:"actorId"`
	ActorDisplayName string `json:"actorDisplayName"`
	Timestamp        int    `json:"timestamp"`
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

	ext := NewNextcloudTalkExtension(cfg)
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
