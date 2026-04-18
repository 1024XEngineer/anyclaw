package ingress

import (
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestServiceDecideChannelPrefersReplyTargetAsRouteSource(t *testing.T) {
	service := NewService(NewRouter(config.RoutingConfig{Mode: "per-chat"}))

	decision := service.DecideChannel(ChannelRequest{
		Channel:     "telegram",
		SessionID:   "session-fallback",
		ReplyTarget: "chat-42",
		Message:     "hello",
		ThreadID:    "thread-7",
	})

	if decision.Key != "telegram:chat-42:thread:thread-7" {
		t.Fatalf("expected reply target to drive routing key, got %q", decision.Key)
	}
}
