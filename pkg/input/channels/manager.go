package channels

import (
	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

func BuildAdapters(cfg config.ChannelsConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) []inputlayer.Adapter {
	return []inputlayer.Adapter{
		NewTelegramAdapter(cfg.Telegram, appendEvent),
		NewSlackAdapter(cfg.Slack, appendEvent),
		NewDiscordAdapter(cfg.Discord, appendEvent),
		NewSignalAdapter(cfg.Signal, appendEvent),
	}
}

func NewManager(cfg config.ChannelsConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) *inputlayer.Manager {
	return inputlayer.NewManager(BuildAdapters(cfg, appendEvent)...)
}
