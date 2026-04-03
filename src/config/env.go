package config

import (
	"os"
	"strconv"
	"strings"
)

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
		cfg.LLM.Provider = "anthropic"
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("ANYCLAW_GATEWAY_HOST"); v != "" {
		cfg.Gateway.Host = v
	}
	if v := os.Getenv("ANYCLAW_GATEWAY_BIND"); v != "" {
		cfg.Gateway.Bind = v
	}
	if v := os.Getenv("ANYCLAW_GATEWAY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.Gateway.Port = port
		}
	}
	if v := os.Getenv("ANYCLAW_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Channels.Telegram.BotToken = v
	}
	if v := os.Getenv("ANYCLAW_TELEGRAM_CHAT_ID"); v != "" {
		cfg.Channels.Telegram.ChatID = v
	}
	if v := os.Getenv("ANYCLAW_SLACK_BOT_TOKEN"); v != "" {
		cfg.Channels.Slack.BotToken = v
	}
	if v := os.Getenv("ANYCLAW_SLACK_APP_TOKEN"); v != "" {
		cfg.Channels.Slack.AppToken = v
	}
	if v := os.Getenv("ANYCLAW_SLACK_DEFAULT_CHANNEL"); v != "" {
		cfg.Channels.Slack.DefaultChannel = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_BOT_TOKEN"); v != "" {
		cfg.Channels.Discord.BotToken = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_DEFAULT_CHANNEL"); v != "" {
		cfg.Channels.Discord.DefaultChannel = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_API_BASE_URL"); v != "" {
		cfg.Channels.Discord.APIBaseURL = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_GUILD_ID"); v != "" {
		cfg.Channels.Discord.GuildID = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_PUBLIC_KEY"); v != "" {
		cfg.Channels.Discord.PublicKey = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_USE_GATEWAY_WS"); v != "" {
		cfg.Channels.Discord.UseGatewayWS = strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_ACCESS_TOKEN"); v != "" {
		cfg.Channels.WhatsApp.AccessToken = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_PHONE_NUMBER_ID"); v != "" {
		cfg.Channels.WhatsApp.PhoneNumberID = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_VERIFY_TOKEN"); v != "" {
		cfg.Channels.WhatsApp.VerifyToken = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_APP_SECRET"); v != "" {
		cfg.Channels.WhatsApp.AppSecret = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_DEFAULT_RECIPIENT"); v != "" {
		cfg.Channels.WhatsApp.DefaultRecipient = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_BASE_URL"); v != "" {
		cfg.Channels.Signal.BaseURL = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_NUMBER"); v != "" {
		cfg.Channels.Signal.Number = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_DEFAULT_RECIPIENT"); v != "" {
		cfg.Channels.Signal.DefaultRecipient = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_BEARER_TOKEN"); v != "" {
		cfg.Channels.Signal.BearerToken = v
	}
	if v := os.Getenv("ANYCLAW_API_TOKEN"); v != "" {
		cfg.Security.APIToken = v
	}
	if v := os.Getenv("ANYCLAW_WEBHOOK_SECRET"); v != "" {
		cfg.Security.WebhookSecret = v
	}
	if v := os.Getenv("ANYCLAW_RATE_LIMIT_RPM"); v != "" {
		if rpm, err := strconv.Atoi(v); err == nil && rpm > 0 {
			cfg.Security.RateLimitRPM = rpm
		}
	}
	if v := os.Getenv("ANYCLAW_PLUGIN_EXEC_TIMEOUT"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Plugins.ExecTimeoutSeconds = sec
		}
	}
}
