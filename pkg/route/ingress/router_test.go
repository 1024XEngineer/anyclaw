package ingress

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestRouterDecideIncludesThreadInConversationKey(t *testing.T) {
	router := NewRouter(config.RoutingConfig{Mode: "per-chat"})

	decision := router.Decide(RouteRequest{
		Channel:  "telegram",
		Source:   "chat-1",
		Text:     "hello",
		ThreadID: "thread-9",
	})

	if decision.RouteKey != "telegram:chat-1:thread:thread-9" {
		t.Fatalf("expected thread-scoped key, got %q", decision.RouteKey)
	}
	if decision.ThreadID != "thread-9" {
		t.Fatalf("expected thread id to be preserved, got %q", decision.ThreadID)
	}
	if decision.SessionMode != "per-chat" {
		t.Fatalf("expected per-chat mode, got %q", decision.SessionMode)
	}
}

func TestRouterDecideAppliesSessionFieldsFromRule(t *testing.T) {
	replyBack := true
	router := NewRouter(config.RoutingConfig{
		Mode: "per-chat",
		Rules: []config.ChannelRoutingRule{
			{
				Channel:     "slack",
				Match:       "deploy",
				SessionMode: "shared",
				SessionID:   "sess-fixed",
				QueueMode:   "fifo",
				ReplyBack:   &replyBack,
				TitlePrefix: "Ops",
				Agent:       "legacy-agent",
				Workspace:   "legacy-workspace",
			},
		},
	})

	decision := router.Decide(RouteRequest{
		Channel: "slack",
		Source:  "channel:user-1",
		Text:    "please deploy",
	})

	if decision.SessionMode != "shared" {
		t.Fatalf("expected shared mode, got %q", decision.SessionMode)
	}
	if decision.ForcedSessionID != "sess-fixed" {
		t.Fatalf("expected fixed session id, got %q", decision.ForcedSessionID)
	}
	if decision.QueueMode != "fifo" {
		t.Fatalf("expected fifo queue mode, got %q", decision.QueueMode)
	}
	if !decision.ReplyBack {
		t.Fatal("expected reply_back to be applied")
	}
	if decision.TitleHint != "Ops channel:user-1" {
		t.Fatalf("expected prefixed title, got %q", decision.TitleHint)
	}
}

func TestRouterDecidePrefersExplicitTitleHint(t *testing.T) {
	router := NewRouter(config.RoutingConfig{Mode: "per-chat"})

	decision := router.Decide(RouteRequest{
		Channel:   "webhook",
		Source:    "ticket-42",
		Text:      "please help",
		TitleHint: "Webhook ticket",
	})

	if decision.TitleHint != "Webhook ticket" {
		t.Fatalf("expected explicit title hint, got %q", decision.TitleHint)
	}
}
