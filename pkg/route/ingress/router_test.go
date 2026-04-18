package ingress

import (
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestRouterDecideIncludesThreadInConversationKey(t *testing.T) {
	router := NewRouter(config.RoutingConfig{Mode: "per-chat"})

	decision := router.Decide(RouteRequest{
		Channel:  "telegram",
		Source:   "chat-1",
		Text:     "hello",
		ThreadID: "thread-9",
	})

	if decision.Key != "telegram:chat-1:thread:thread-9" {
		t.Fatalf("expected thread-scoped key, got %q", decision.Key)
	}
	if !decision.IsThread {
		t.Fatal("expected decision to mark thread routing")
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
	if decision.SessionID != "sess-fixed" {
		t.Fatalf("expected fixed session id, got %q", decision.SessionID)
	}
	if decision.QueueMode != "fifo" {
		t.Fatalf("expected fifo queue mode, got %q", decision.QueueMode)
	}
	if !decision.ReplyBack {
		t.Fatal("expected reply_back to be applied")
	}
	if decision.Title != "Ops channel:user-1" {
		t.Fatalf("expected prefixed title, got %q", decision.Title)
	}
}
