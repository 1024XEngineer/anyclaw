package ingress

import "testing"

func TestDeliveryResolverCopiesTransportFacts(t *testing.T) {
	resolver := DeliveryResolver{}

	target := resolver.Resolve(MainRouteRequest{
		Scope: MessageScope{
			ChannelID:      "discord",
			ConversationID: "room-8",
			ThreadID:       "thread-2",
			Metadata: map[string]string{
				"guild_id": "guild-1",
			},
		},
		DeliveryHint: DeliveryHint{
			ReplyTo: "reply-room-8",
			Metadata: map[string]string{
				"chat_id": "room-8",
			},
		},
	}, SessionSnapshot{
		ConversationKey: "discord:room-8:thread:thread-2",
	})

	if target.ChannelID != "discord" {
		t.Fatalf("expected discord delivery channel, got %q", target.ChannelID)
	}
	if target.ConversationID != "room-8" {
		t.Fatalf("expected room-8 delivery conversation, got %q", target.ConversationID)
	}
	if target.ReplyTo != "reply-room-8" {
		t.Fatalf("expected reply-room-8 delivery reply target, got %q", target.ReplyTo)
	}
	if target.ThreadID != "thread-2" {
		t.Fatalf("expected thread-2 delivery thread, got %q", target.ThreadID)
	}
	if target.TransportMeta["channel_id"] != "discord" || target.TransportMeta["chat_id"] != "room-8" || target.TransportMeta["conversation_key"] != "discord:room-8:thread:thread-2" {
		t.Fatalf("expected delivery metadata to preserve transport facts, got %#v", target.TransportMeta)
	}
}
