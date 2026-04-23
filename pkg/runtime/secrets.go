package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
)

// buildInitialSecretsSnapshot only prepares the in-memory bootstrap snapshot.
// Callers can persist secrets later once they have an explicitly configured store policy.
func buildInitialSecretsSnapshot(_ *secrets.Store, cfg *config.Config) *secrets.RuntimeSnapshot {
	entries := make(map[string]*secrets.SecretEntry)
	now := time.Now().UTC()

	if cfg == nil {
		return secrets.NewRuntimeSnapshot(entries, "bootstrap")
	}

	seedSecret := func(key string, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		entries[key] = &secrets.SecretEntry{
			ID:        fmt.Sprintf("sec-%d", time.Now().UnixNano()),
			Key:       key,
			Value:     value,
			Scope:     secrets.ScopeGlobal,
			Source:    secrets.SourceManual,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	seedSecret("llm_api_key", cfg.LLM.APIKey)
	seedSecret("security_api_token", cfg.Security.APIToken)
	seedSecret("security_webhook_secret", cfg.Security.WebhookSecret)

	for _, p := range cfg.Providers {
		if strings.TrimSpace(p.APIKey) != "" {
			seedSecret("provider_"+p.ID+"_api_key", p.APIKey)
		}
	}

	if strings.TrimSpace(cfg.Channels.Telegram.BotToken) != "" {
		seedSecret("channel_telegram_bot_token", cfg.Channels.Telegram.BotToken)
	}
	if strings.TrimSpace(cfg.Channels.Slack.BotToken) != "" {
		seedSecret("channel_slack_bot_token", cfg.Channels.Slack.BotToken)
	}
	if strings.TrimSpace(cfg.Channels.Discord.BotToken) != "" {
		seedSecret("channel_discord_bot_token", cfg.Channels.Discord.BotToken)
	}
	if strings.TrimSpace(cfg.Channels.WhatsApp.AccessToken) != "" {
		seedSecret("channel_whatsapp_access_token", cfg.Channels.WhatsApp.AccessToken)
	}
	if strings.TrimSpace(cfg.Channels.Signal.BearerToken) != "" {
		seedSecret("channel_signal_bearer_token", cfg.Channels.Signal.BearerToken)
	}

	snap := secrets.NewRuntimeSnapshot(entries, "bootstrap")
	return snap
}

func resolveSecret(snap *secrets.RuntimeSnapshot, plaintext string, secretKey string) string {
	if snap != nil {
		if strings.Contains(plaintext, "${SECRET:") {
			resolved, err := snap.ResolveValueStrict(plaintext)
			if err == nil {
				return resolved
			}
		}
		if entry, ok := snap.Get(secretKey); ok {
			return entry.Value
		}
	}
	return plaintext
}
