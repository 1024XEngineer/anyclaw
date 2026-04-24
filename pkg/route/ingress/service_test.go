package ingress

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestServiceDecideChannelPrefersReplyTargetAsRouteSource(t *testing.T) {
	service := NewService(NewRouter(config.RoutingConfig{Mode: "per-chat"}))

	decision, err := service.DecideChannel(ChannelRequest{
		Channel:     "telegram",
		SessionID:   "session-fallback",
		ReplyTarget: "chat-42",
		Message:     "hello",
		ThreadID:    "thread-7",
	})
	if err != nil {
		t.Fatalf("DecideChannel: %v", err)
	}

	if decision.Key != "telegram:chat-42:thread:thread-7" {
		t.Fatalf("expected reply target to drive routing key, got %q", decision.Key)
	}
}

func TestServiceDecideChannelReturnsProjectorError(t *testing.T) {
	service := NewService(NewRouter(config.RoutingConfig{Mode: "per-chat"}))

	_, err := service.DecideChannel(ChannelRequest{})
	if err == nil {
		t.Fatal("expected DecideChannel to return projector error")
	}
}

func TestServiceRouteRunsFullIngressChain(t *testing.T) {
	store := &stubSessionStore{
		sessionsByRouteKey: map[string]SessionSnapshot{
			"telegram:chat-22": {
				ID:              "sess-22",
				ConversationKey: "telegram:chat-22",
				ReplyTarget:     "chat-22",
			},
		},
	}
	service := NewService(
		NewRouter(config.RoutingConfig{Mode: "per-chat"}),
		WithMainAgentNameResolver(func() string { return "AnyClaw" }),
		WithSessionStore(store),
	)

	output, err := service.Route(RouteInput{
		Entry: IngressRoutingEntry{
			Text: "reuse this session",
			Scope: MessageScope{
				ChannelID:      "telegram",
				ConversationID: "chat-22",
			},
		},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}

	if output.Request.Route.Agent.AgentName != "AnyClaw" {
		t.Fatalf("expected AnyClaw agent, got %#v", output.Request.Route.Agent)
	}
	if output.Request.Route.Session.SessionID != "sess-22" {
		t.Fatalf("expected routed session id sess-22, got %q", output.Request.Route.Session.SessionID)
	}
	if output.Request.Route.Session.MatchedBy != "conversation_key" {
		t.Fatalf("expected conversation_key session match, got %q", output.Request.Route.Session.MatchedBy)
	}
	if output.Request.Route.Delivery.ConversationID != "chat-22" {
		t.Fatalf("expected delivery conversation chat-22, got %q", output.Request.Route.Delivery.ConversationID)
	}
	if output.Request.Route.Delivery.TransportMeta["conversation_key"] != "telegram:chat-22" {
		t.Fatalf("expected delivery metadata conversation_key telegram:chat-22, got %#v", output.Request.Route.Delivery.TransportMeta)
	}
}
