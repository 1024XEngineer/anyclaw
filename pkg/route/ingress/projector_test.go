package ingress

import "testing"

func TestProjectorNormalizesIngressRoutingEntry(t *testing.T) {
	projector := IngressRouteProjector{}

	request, err := projector.Project(IngressRoutingEntry{
		MessageID: "msg-1",
		Text:      "hello from telegram",
		Actor: MessageActor{
			UserID: "user-1",
		},
		Scope: MessageScope{
			ChannelID: "telegram",
			Metadata: map[string]string{
				"username": "alice",
			},
		},
		Delivery: DeliveryHint{
			ConversationID: "chat-42",
			ReplyTo:        "reply-9",
			ThreadID:       "thread-7",
		},
		Hint: RouteHint{
			RequestedSessionID: "session-hint",
			TitleHint:          "Webhook ticket",
		},
	})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if request.Scope.EntryPoint != "channel" {
		t.Fatalf("expected default entry point channel, got %q", request.Scope.EntryPoint)
	}
	if request.Scope.ConversationID != "chat-42" {
		t.Fatalf("expected conversation id from delivery hint, got %q", request.Scope.ConversationID)
	}
	if request.DeliveryHint.ChannelID != "telegram" {
		t.Fatalf("expected delivery channel telegram, got %q", request.DeliveryHint.ChannelID)
	}
	if request.DeliveryHint.ReplyTo != "reply-9" {
		t.Fatalf("expected reply target reply-9, got %q", request.DeliveryHint.ReplyTo)
	}
	if request.Scope.ThreadID != "thread-7" {
		t.Fatalf("expected thread id thread-7, got %q", request.Scope.ThreadID)
	}
	if request.Actor.DisplayName != "alice" {
		t.Fatalf("expected display name alice, got %q", request.Actor.DisplayName)
	}
	if request.Hint.RequestedSessionID != "session-hint" {
		t.Fatalf("expected requested session hint to survive projection, got %q", request.Hint.RequestedSessionID)
	}
	if request.Hint.TitleHint != "Webhook ticket" {
		t.Fatalf("expected title hint to survive projection, got %q", request.Hint.TitleHint)
	}
}

func TestProjectorRequiresEntryPointOrChannel(t *testing.T) {
	projector := IngressRouteProjector{}

	_, err := projector.Project(IngressRoutingEntry{})
	if err == nil {
		t.Fatal("expected projector to reject empty ingress routing entry")
	}
}
