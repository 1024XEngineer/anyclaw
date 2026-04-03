package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/config"
)

type Adapter struct {
	base     channel.BaseAdapter
	config   config.DiscordChannelConfig
	client   *http.Client
	handler  channel.InboundHandler
	mu       sync.Mutex
	sessions map[string]string
}

func NewAdapter(cfg config.DiscordChannelConfig) *Adapter {
	return &Adapter{
		base:     channel.NewBaseAdapter("discord", cfg.Enabled && cfg.BotToken != ""),
		config:   cfg,
		client:   &http.Client{Timeout: 20 * time.Second},
		sessions: make(map[string]string),
	}
}

func (a *Adapter) SetHandler(handler channel.InboundHandler) {
	a.handler = handler
}

func (a *Adapter) Name() string {
	return "discord"
}

func (a *Adapter) Enabled() bool {
	return a.config.Enabled && a.config.BotToken != ""
}

func (a *Adapter) Run(ctx context.Context, handler channel.InboundHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)

	a.base.MarkActivity()
	<-ctx.Done()
	return nil
}

func (a *Adapter) HandleInteraction(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	payloadType, _ := payload["type"].(float64)
	if payloadType == 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"type": 1})
		return
	}

	if payloadType == 2 {
		data, _ := payload["data"].(map[string]any)
		options, _ := data["options"].([]any)

		var message string
		if len(options) > 0 {
			if opt, ok := options[0].(map[string]any); ok {
				message, _ = opt["value"].(string)
			}
		}

		user, _ := payload["member"].(map[string]any)
		username, _ := user["user"].(map[string]any)["username"].(string)

		meta := map[string]string{
			"channel":  "discord",
			"username": username,
		}

		_, response, _ := a.handler(context.Background(), "", message, meta)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"type": 4,
			"data": map[string]any{
				"content": response,
			},
		})
	}
}

func (a *Adapter) SendMessage(ctx context.Context, channelID, message string) error {
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	body, _ := json.Marshal(map[string]string{"content": message})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord API error: %s", resp.Status)
	}

	return nil
}

func (a *Adapter) Status() channel.Status {
	return a.base.Status()
}
