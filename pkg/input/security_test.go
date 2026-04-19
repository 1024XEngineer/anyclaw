package input

import (
	"context"
	"strings"
	"testing"
)

func TestChannelPolicyWrapAllowsGroupMessagesWhenOnlyDMAllowListIsConfigured(t *testing.T) {
	policy := DefaultChannelPolicy()
	policy.SetDMPolicy(DMPolicyAllowList)
	policy.SetGroupPolicy(GroupPolicyAllowAll)
	policy.AddAllowedUser("dm-user")

	called := false
	wrapped := policy.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		called = true
		return sessionID, "ok", nil
	})

	_, _, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"channel":    "discord",
		"guild_id":   "guild-1",
		"channel_id": "channel-1",
		"user_id":    "group-user",
	})
	if err != nil {
		t.Fatalf("expected group message to pass, got %v", err)
	}
	if !called {
		t.Fatal("expected wrapped handler to be called")
	}
}

func TestChannelPolicyWrapAppliesDMPolicyToTelegramPrivateChatFallback(t *testing.T) {
	policy := DefaultChannelPolicy()
	policy.SetDMPolicy(DMPolicyAllowList)
	policy.AddAllowedUser("allowed-user")

	wrapped := policy.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		t.Fatal("handler should not be called for blocked DM")
		return "", "", nil
	})

	_, _, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"channel": "telegram",
		"chat_id": "42",
		"user_id": "42",
	})
	if err == nil {
		t.Fatal("expected DM policy to block unlisted user")
	}
	if !strings.Contains(err.Error(), "blocked by DM policy") {
		t.Fatalf("expected DM policy error, got %v", err)
	}
}
