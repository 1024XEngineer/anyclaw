package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type GoogleChatConfig struct {
	Enabled    bool   `json:"enabled"`
	ProjectID  string `json:"project_id"`
	SpaceID    string `json:"space_id"`
	Credential string `json:"credential"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type GoogleChatAdapter struct {
	BaseAdapter
	config  GoogleChatConfig
	handler InboundHandler
	mu      sync.RWMutex
}

func NewGoogleChatAdapter(cfg GoogleChatConfig) *GoogleChatAdapter {
	return &GoogleChatAdapter{
		BaseAdapter: NewBaseAdapter("googlechat", cfg.Enabled),
		config:      cfg,
	}
}

func (a *GoogleChatAdapter) Enabled() bool {
	return a.config.Enabled
}

func (a *GoogleChatAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if !a.config.Enabled {
		return nil
	}
	a.handler = handle
	a.setRunning(true)
	defer a.setRunning(false)
	a.setError(nil)
	a.markActivity()

	<-ctx.Done()
	return ctx.Err()
}

func (a *GoogleChatAdapter) HandleWebhook(ctx context.Context, body []byte) (map[string]any, error) {
	var payload struct {
		Type      string `json:"type"`
		EventTime string `json:"eventTime"`
		Message   struct {
			Name   string `json:"name"`
			Sender struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"sender"`
			Text  string `json:"text"`
			Space struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"space"`
			Thread struct {
				Name string `json:"name"`
			} `json:"thread"`
		} `json:"message"`
		Space struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"space"`
		Config struct {
			DestinationID string `json:"customResponseId"`
		} `json:"config"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if payload.Type != "MESSAGE" {
		return map[string]any{"text": ""}, nil
	}

	senderID := payload.Message.Sender.Name
	senderName := payload.Message.Sender.DisplayName
	text := payload.Message.Text
	spaceID := payload.Space.Name

	sessionID := fmt.Sprintf("gchat:%s:%s", spaceID, senderID)

	if a.handler != nil {
		meta := map[string]string{
			"channel":  "googlechat",
			"user_id":  senderID,
			"username": senderName,
			"chat_id":  spaceID,
		}
		response, _, err := a.handler(ctx, sessionID, text, meta)
		if err != nil {
			return nil, err
		}
		a.markActivity()
		return map[string]any{"text": response}, nil
	}

	return map[string]any{"text": ""}, nil
}

// MS Teams Config
type MSTeamsConfig struct {
	Enabled     bool   `json:"enabled"`
	AppID       string `json:"app_id"`
	AppPassword string `json:"app_password"`
	TenantID    string `json:"tenant_id"`
	ServiceURL  string `json:"service_url"`
	WebhookPath string `json:"webhook_path"`
}

type MSTeamsAdapter struct {
	BaseAdapter
	config  MSTeamsConfig
	handler InboundHandler
	mu      sync.RWMutex
}

func NewMSTeamsAdapter(cfg MSTeamsConfig) *MSTeamsAdapter {
	return &MSTeamsAdapter{
		BaseAdapter: NewBaseAdapter("msteams", cfg.Enabled),
		config:      cfg,
	}
}

func (a *MSTeamsAdapter) Enabled() bool {
	return a.config.Enabled
}

func (a *MSTeamsAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if !a.config.Enabled {
		return nil
	}
	a.handler = handle
	a.setRunning(true)
	defer a.setRunning(false)
	a.setError(nil)
	a.markActivity()

	<-ctx.Done()
	return ctx.Err()
}

func (a *MSTeamsAdapter) HandleWebhook(ctx context.Context, body []byte) (map[string]any, error) {
	var payload struct {
		Type string `json:"type"`
		Text string `json:"text"`
		From struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			AadObjectID string `json:"aadObjectId"`
		} `json:"from"`
		Conversation struct {
			ID               string `json:"id"`
			IsGroup          bool   `json:"isGroup"`
			ConversationType string `json:"conversationType"`
		} `json:"conversation"`
		ChannelID  string `json:"channelId"`
		ServiceURL string `json:"serviceURL"`
		ReplyToID  string `json:"replyToId"`
		ID         string `json:"id"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if payload.Type != "message" || payload.Text == "" {
		return nil, nil
	}

	senderID := payload.From.ID
	senderName := payload.From.Name
	text := payload.Text
	conversationID := payload.Conversation.ID

	sessionID := fmt.Sprintf("teams:%s:%s", conversationID, senderID)

	if a.handler != nil {
		meta := map[string]string{
			"channel":  "msteams",
			"user_id":  senderID,
			"username": senderName,
			"chat_id":  conversationID,
		}
		response, _, err := a.handler(ctx, sessionID, text, meta)
		if err != nil {
			return nil, err
		}
		a.markActivity()
		return map[string]any{
			"type": "message",
			"text": response,
		}, nil
	}

	return nil, nil
}

// Matrix Config
type MatrixConfig struct {
	Enabled     bool   `json:"enabled"`
	Homeserver  string `json:"homeserver"`
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	RoomID      string `json:"room_id"`
	DeviceID    string `json:"device_id,omitempty"`
}

type MatrixAdapter struct {
	BaseAdapter
	config    MatrixConfig
	handler   InboundHandler
	mu        sync.RWMutex
	client    *http.Client
	syncToken string
}

func NewMatrixAdapter(cfg MatrixConfig) *MatrixAdapter {
	return &MatrixAdapter{
		BaseAdapter: NewBaseAdapter("matrix", cfg.Enabled),
		config:      cfg,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *MatrixAdapter) Enabled() bool {
	return a.config.Enabled
}

func (a *MatrixAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if !a.config.Enabled {
		return nil
	}
	a.handler = handle
	a.setRunning(true)
	defer a.setRunning(false)
	a.setError(nil)

	// Matrix sync loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := a.sync(ctx); err != nil {
			a.setError(err)
			time.Sleep(5 * time.Second)
			continue
		}
		a.markActivity()
	}
}

func (a *MatrixAdapter) sync(ctx context.Context) error {
	syncURL := fmt.Sprintf("%s/_matrix/client/r0/sync", a.config.Homeserver)
	if a.syncToken != "" {
		syncURL += "?since=" + a.syncToken
	}

	req, err := http.NewRequestWithContext(ctx, "GET", syncURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.AccessToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
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

	a.syncToken = syncResp.NextBatch

	for roomID, room := range syncResp.Rooms.Join {
		for _, event := range room.Timeline.Events {
			if event.Type == "m.room.message" && event.Content.MsgType == "m.text" {
				if a.handler != nil && event.Sender != a.config.UserID {
					sessionID := fmt.Sprintf("matrix:%s:%s", roomID, event.Sender)
					meta := map[string]string{
						"channel": "matrix",
						"user_id": event.Sender,
						"chat_id": roomID,
					}
					response, _, err := a.handler(ctx, sessionID, event.Content.Body, meta)
					if err == nil && response != "" {
						a.sendMatrixMessage(ctx, roomID, response)
					}
				}
			}
		}
	}

	return nil
}

func (a *MatrixAdapter) sendMatrixMessage(ctx context.Context, roomID, text string) error {
	txnID := fmt.Sprintf("m%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/_matrix/client/r0/rooms/%s/send/m.room.message/%s", a.config.Homeserver, roomID, txnID)

	payload := map[string]any{
		"msgtype": "m.text",
		"body":    text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// LINE Config
type LINEConfig struct {
	Enabled       bool   `json:"enabled"`
	ChannelSecret string `json:"channel_secret"`
	ChannelToken  string `json:"channel_token"`
	WebhookPath   string `json:"webhook_path"`
}

type LINEAdapter struct {
	BaseAdapter
	config  LINEConfig
	handler InboundHandler
	mu      sync.RWMutex
}

func NewLINEAdapter(cfg LINEConfig) *LINEAdapter {
	return &LINEAdapter{
		BaseAdapter: NewBaseAdapter("line", cfg.Enabled),
		config:      cfg,
	}
}

func (a *LINEAdapter) Enabled() bool {
	return a.config.Enabled
}

func (a *LINEAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if !a.config.Enabled {
		return nil
	}
	a.handler = handle
	a.setRunning(true)
	defer a.setRunning(false)
	a.setError(nil)
	a.markActivity()

	<-ctx.Done()
	return ctx.Err()
}

func (a *LINEAdapter) HandleWebhook(ctx context.Context, body []byte) (map[string]any, error) {
	var payload struct {
		Events []struct {
			Type       string `json:"type"`
			ReplyToken string `json:"replyToken"`
			Source     struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
			} `json:"source"`
			Message struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"events"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	for _, event := range payload.Events {
		if event.Type != "message" || event.Message.Type != "text" {
			continue
		}

		senderID := event.Source.UserID
		chatID := event.Source.UserID
		if event.Source.GroupID != "" {
			chatID = event.Source.GroupID
		}

		sessionID := fmt.Sprintf("line:%s:%s", chatID, senderID)

		if a.handler != nil {
			meta := map[string]string{
				"channel": "line",
				"user_id": senderID,
				"chat_id": chatID,
			}
			response, _, err := a.handler(ctx, sessionID, event.Message.Text, meta)
			if err == nil && response != "" && event.ReplyToken != "" {
				a.replyLINE(event.ReplyToken, response)
			}
		}
	}

	return nil, nil
}

func (a *LINEAdapter) replyLINE(replyToken, text string) error {
	url := "https://api.line.me/v2/bot/message/reply"
	payload := map[string]any{
		"replyToken": replyToken,
		"messages": []map[string]any{
			{"type": "text", "text": text},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.ChannelToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Mattermost Config
type MattermostConfig struct {
	Enabled    bool   `json:"enabled"`
	ServerURL  string `json:"server_url"`
	TeamID     string `json:"team_id"`
	ChannelID  string `json:"channel_id"`
	BotToken   string `json:"bot_token"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type MattermostAdapter struct {
	BaseAdapter
	config  MattermostConfig
	handler InboundHandler
	mu      sync.RWMutex
	client  *http.Client
}

func NewMattermostAdapter(cfg MattermostConfig) *MattermostAdapter {
	return &MattermostAdapter{
		BaseAdapter: NewBaseAdapter("mattermost", cfg.Enabled),
		config:      cfg,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *MattermostAdapter) Enabled() bool {
	return a.config.Enabled
}

func (a *MattermostAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if !a.config.Enabled {
		return nil
	}
	a.handler = handle
	a.setRunning(true)
	defer a.setRunning(false)
	a.setError(nil)

	// WebSocket connection for real-time events
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := a.connectWebSocket(ctx); err != nil {
			a.setError(err)
			time.Sleep(5 * time.Second)
			continue
		}
		a.markActivity()
	}
}

func (a *MattermostAdapter) connectWebSocket(ctx context.Context) error {
	// Mattermost WebSocket implementation would go here
	// For now, we'll use webhook mode
	<-ctx.Done()
	return ctx.Err()
}

func (a *MattermostAdapter) HandleWebhook(ctx context.Context, body []byte) (map[string]any, error) {
	var payload struct {
		Token     string `json:"token"`
		TeamID    string `json:"team_id"`
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
		UserName  string `json:"user_name"`
		Text      string `json:"text"`
		PostID    string `json:"post_id"`
		Type      string `json:"type"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if payload.Type != "" && payload.Type != "event" {
		return nil, nil
	}

	sessionID := fmt.Sprintf("mattermost:%s:%s", payload.ChannelID, payload.UserID)

	if a.handler != nil {
		meta := map[string]string{
			"channel":  "mattermost",
			"user_id":  payload.UserID,
			"username": payload.UserName,
			"chat_id":  payload.ChannelID,
		}
		response, _, err := a.handler(ctx, sessionID, payload.Text, meta)
		if err != nil {
			return nil, err
		}
		a.markActivity()

		// Post response to Mattermost
		if response != "" {
			a.postMessage(payload.ChannelID, response)
		}
	}

	return nil, nil
}

func (a *MattermostAdapter) postMessage(channelID, text string) error {
	url := fmt.Sprintf("%s/api/v4/posts", a.config.ServerURL)
	payload := map[string]any{
		"channel_id": channelID,
		"message":    text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
