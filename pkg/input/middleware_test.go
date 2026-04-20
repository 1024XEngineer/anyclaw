package input

import (
	"context"
	"strings"
	"testing"
)

func TestChannelCommandsWrapStreamEmitsCommandOutput(t *testing.T) {
	cc := NewChannelCommands("AnyClaw")
	wrapped := cc.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		t.Fatal("stream handler should not be called for built-in commands")
		return "", nil
	})

	var chunks []string
	sessionID, err := wrapped(context.Background(), "session-1", "/help", map[string]string{"channel": "slack"}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("wrap stream returned error: %v", err)
	}
	if sessionID != "session-1" {
		t.Fatalf("expected session to be preserved, got %q", sessionID)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected one streamed command response, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "Available commands:") {
		t.Fatalf("expected streamed help output, got %q", chunks[0])
	}
}

func TestMentionGateBlocksSlackChannelMessagesWithoutMentionFallback(t *testing.T) {
	gate := NewMentionGate(true, "BOT123", nil)

	if gate.ShouldProcess("hello team", map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	}) {
		t.Fatal("expected slack channel message without mention to be blocked")
	}
}

func TestMentionGateBlocksTelegramGroupMessagesWithoutMentionFallback(t *testing.T) {
	gate := NewMentionGate(true, "bot", nil)

	if gate.ShouldProcess("hello group", map[string]string{
		"channel":   "telegram",
		"chat_id":   "-100123",
		"chat_type": "supergroup",
	}) {
		t.Fatal("expected telegram group message without mention to be blocked")
	}
}
