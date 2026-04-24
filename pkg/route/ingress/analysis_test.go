package ingress

import (
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestAnalyzeRoutingFlagsShadowedAndBroadRules(t *testing.T) {
	replyBack := true
	warnings := AnalyzeRouting(config.RoutingConfig{
		Rules: []config.ChannelRoutingRule{
			{
				Channel:   "telegram",
				Match:     "",
				ReplyBack: &replyBack,
			},
			{
				Channel: "telegram",
				Match:   "deploy",
			},
		},
	})

	if len(warnings) != 2 {
		t.Fatalf("expected two warnings, got %#v", warnings)
	}
	if warnings[0].Kind != "broad" {
		t.Fatalf("expected first warning to be broad, got %#v", warnings[0])
	}
	if warnings[1].Kind != "shadowed" {
		t.Fatalf("expected second warning to be shadowed, got %#v", warnings[1])
	}
}

func TestAnalyzeRoutingFlagsDuplicateRules(t *testing.T) {
	warnings := AnalyzeRouting(config.RoutingConfig{
		Rules: []config.ChannelRoutingRule{
			{
				Channel:     "slack",
				Match:       "deploy",
				SessionMode: "shared",
				SessionID:   "sess-1",
				QueueMode:   "fifo",
				TitlePrefix: "Ops",
			},
			{
				Channel:     "slack",
				Match:       "deploy",
				SessionMode: "shared",
				SessionID:   "sess-1",
				QueueMode:   "fifo",
				TitlePrefix: "Ops",
			},
		},
	})

	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", warnings)
	}
	if warnings[0].Kind != "duplicate" {
		t.Fatalf("expected duplicate warning, got %#v", warnings[0])
	}
}

func TestAnalyzeRoutingUsesRouterChannelAndDefaultModeSemantics(t *testing.T) {
	warnings := AnalyzeRouting(config.RoutingConfig{
		Mode: "shared",
		Rules: []config.ChannelRoutingRule{
			{
				Channel: "Telegram ",
				Match:   "deploy",
			},
			{
				Channel:     "telegram",
				Match:       "deploy",
				SessionMode: "shared",
			},
		},
	})

	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", warnings)
	}
	if warnings[0].Kind != "duplicate" {
		t.Fatalf("expected duplicate warning, got %#v", warnings[0])
	}
}

func TestAnalyzeRoutingKeepsRawMatchSemantics(t *testing.T) {
	warnings := AnalyzeRouting(config.RoutingConfig{
		Rules: []config.ChannelRoutingRule{
			{
				Channel: "telegram",
				Match:   " deploy",
			},
			{
				Channel: "telegram",
				Match:   "deploy now",
			},
		},
	})

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
}
