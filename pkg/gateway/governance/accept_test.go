package governance

import (
	"context"
	"testing"
)

func TestAcceptBuildsNormalizedRequest(t *testing.T) {
	service := Service{}

	request, err := service.Accept(context.Background(), RawRequest{
		SourceType:          "channel",
		EntryPoint:          "channel",
		ChannelID:           "telegram",
		SessionID:           "chat-42",
		Message:             "hello",
		ActorUserID:         "user-7",
		ActorDisplayName:    "alice",
		RequestedAgentName:  "ops-agent",
		RequestedSessionID:  "sess-1",
		DeliveryReplyTarget: "reply-9",
		ThreadID:            "thread-3",
		GroupID:             "guild-1",
		IsGroup:             true,
		Metadata: map[string]string{
			"channel_id": "telegram",
		},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if request.SourceType != "channel" {
		t.Fatalf("expected source type channel, got %q", request.SourceType)
	}
	if request.RouteContext.ChannelID != "telegram" {
		t.Fatalf("expected channel telegram, got %q", request.RouteContext.ChannelID)
	}
	if request.RouteContext.ConversationID != "reply-9" {
		t.Fatalf("expected conversation reply-9, got %q", request.RouteContext.ConversationID)
	}
	if request.RouteContext.ThreadID != "thread-3" {
		t.Fatalf("expected thread-3, got %q", request.RouteContext.ThreadID)
	}
	if !request.RouteContext.IsGroup {
		t.Fatal("expected group ingress")
	}
	if request.Actor.UserID != "user-7" {
		t.Fatalf("expected actor user-7, got %q", request.Actor.UserID)
	}
	if request.RequestedAgentName != "ops-agent" {
		t.Fatalf("expected requested agent ops-agent, got %q", request.RequestedAgentName)
	}
	if request.RequestedSessionID != "sess-1" {
		t.Fatalf("expected requested session sess-1, got %q", request.RequestedSessionID)
	}
}
