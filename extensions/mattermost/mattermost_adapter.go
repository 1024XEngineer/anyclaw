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
	ServerURL    string `json:"server_url"`
	BotToken     string `json:"bot_token"`
	TeamID       string `json:"team_id,omitempty"`
	PollInterval int    `json:"poll_interval"`
}

type MattermostExtension struct {
	config     Config
	client     *http.Client
	baseURL    string
	lastPostID map[string]string
}

func NewMattermostExtension(cfg Config) *MattermostExtension {
	serverURL := strings.TrimRight(cfg.ServerURL, "/")
	return &MattermostExtension{
		config:     cfg,
		client:     &http.Client{Timeout: 30 * time.Second},
		baseURL:    serverURL,
		lastPostID: make(map[string]string),
	}
}

func (e *MattermostExtension) Run(ctx context.Context) error {
	interval := time.Duration(e.config.PollInterval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}

	if err := e.initChannels(ctx); err != nil {
		return fmt.Errorf("failed to initialize channels: %w", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := e.pollOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "mattermost poll error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (e *MattermostExtension) initChannels(ctx context.Context) error {
	if e.config.TeamID == "" {
		return nil
	}

	channels, err := e.listChannels(ctx)
	if err != nil {
		return err
	}

	for _, ch := range channels {
		e.lastPostID[ch.ID] = ""
	}

	return nil
}

func (e *MattermostExtension) listChannels(ctx context.Context) ([]Channel, error) {
	url := fmt.Sprintf("%s/api/v4/users/me/teams/%s/channels", e.baseURL, e.config.TeamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.BotToken)

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
		return nil, fmt.Errorf("list channels failed: %s: %s", resp.Status, string(body))
	}

	var channels []Channel
	if err := json.Unmarshal(body, &channels); err != nil {
		return nil, err
	}

	return channels, nil
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	TeamID      string `json:"team_id"`
}

type Post struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Message   string `json:"message"`
	CreateAt  int64  `json:"create_at"`
}

type PostsResponse struct {
	Order      []string         `json:"order"`
	Posts      map[string]*Post `json:"posts"`
	NextPostID string           `json:"next_post_id"`
	PrevPostID string           `json:"prev_post_id"`
}

func (e *MattermostExtension) pollOnce(ctx context.Context) error {
	channels, err := e.listChannels(ctx)
	if err != nil {
		return err
	}

	for _, ch := range channels {
		if ch.Type != "O" && ch.Type != "P" && ch.Type != "D" {
			continue
		}

		posts, err := e.fetchPosts(ctx, ch.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to fetch posts for channel %s: %v\n", ch.ID, err)
			continue
		}

		for _, postID := range posts.Order {
			post, ok := posts.Posts[postID]
			if !ok {
				continue
			}

			if post.Message == "" {
				continue
			}

			if e.lastPostID[ch.ID] != "" && post.ID == e.lastPostID[ch.ID] {
				break
			}

			if e.lastPostID[ch.ID] != "" {
				input := map[string]any{
					"action":   "message",
					"channel":  "mattermost",
					"chat_id":  ch.ID,
					"text":     post.Message,
					"user_id":  post.UserID,
					"username": "",
				}
				data, _ := json.Marshal(input)
				fmt.Println(string(data))

				var response map[string]any
				if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}

				if reply, ok := response["text"].(string); ok && reply != "" {
					if err := e.sendMessage(ctx, ch.ID, reply); err != nil {
						return err
					}
				}
			}

			if e.lastPostID[ch.ID] == "" || isNewerPost(post.ID, e.lastPostID[ch.ID], posts.Order) {
				e.lastPostID[ch.ID] = post.ID
			}
		}
	}

	return nil
}

func isNewerPost(postID, lastID string, order []string) bool {
	lastIdx := -1
	postIdx := -1
	for i, id := range order {
		if id == lastID {
			lastIdx = i
		}
		if id == postID {
			postIdx = i
		}
	}
	if postIdx == -1 {
		return false
	}
	if lastIdx == -1 {
		return true
	}
	return postIdx < lastIdx
}

func (e *MattermostExtension) fetchPosts(ctx context.Context, channelID string) (*PostsResponse, error) {
	url := fmt.Sprintf("%s/api/v4/channels/%s/posts", e.baseURL, channelID)
	if lastID, ok := e.lastPostID[channelID]; ok && lastID != "" {
		url += fmt.Sprintf("?after=%s", lastID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.BotToken)

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
		return nil, fmt.Errorf("fetch posts failed: %s: %s", resp.Status, string(body))
	}

	var postsResp PostsResponse
	if err := json.Unmarshal(body, &postsResp); err != nil {
		return nil, err
	}

	return &postsResp, nil
}

func (e *MattermostExtension) sendMessage(ctx context.Context, channelID, text string) error {
	url := fmt.Sprintf("%s/api/v4/posts", e.baseURL)

	payload := map[string]string{
		"channel_id": channelID,
		"message":    text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.BotToken)
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

	ext := NewMattermostExtension(cfg)
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
