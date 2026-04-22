package ingress

import (
	"testing"

	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
)

func TestPrepareSignedEntryBuildsWebhookIngressEntry(t *testing.T) {
	service := Service{}

	entry := service.PrepareSignedEntry(SignedInput{
		SessionID: "sess-1",
		Title:     "Webhook ticket",
		Message:   "please help",
		Meta: map[string]string{
			"user_id": "user-7",
		},
	})

	if entry.Scope.EntryPoint != "webhook" {
		t.Fatalf("expected webhook entry point, got %q", entry.Scope.EntryPoint)
	}
	if entry.Scope.ChannelID != "webhook" {
		t.Fatalf("expected webhook channel id, got %q", entry.Scope.ChannelID)
	}
	if entry.Hint.RequestedSessionID != "sess-1" {
		t.Fatalf("expected requested session sess-1, got %q", entry.Hint.RequestedSessionID)
	}
	if entry.Hint.TitleHint != "Webhook ticket" {
		t.Fatalf("expected title hint Webhook ticket, got %q", entry.Hint.TitleHint)
	}
	if entry.Actor.UserID != "user-7" {
		t.Fatalf("expected actor user user-7, got %q", entry.Actor.UserID)
	}
}

func TestPrepareNormalizedEntryBuildsTrustedRouteEntry(t *testing.T) {
	service := Service{}

	entry := service.PrepareNormalizedEntry(gatewaygovernance.NormalizedRequest{
		RequestID:   "req-1",
		ContentText: "please help",
		TitleHint:   "Webhook ticket",
		Actor: gatewaygovernance.ActorRef{
			UserID:      "user-8",
			DisplayName: "bob",
		},
		RouteContext: gatewaygovernance.RouteContext{
			EntryPoint:     "webhook",
			SourceType:     "webhook",
			ChannelID:      "webhook",
			ConversationID: "ticket-42",
			ThreadID:       "thread-7",
			Delivery: gatewaygovernance.DeliveryHint{
				ReplyTarget: "ticket-42",
				ThreadID:    "thread-7",
			},
		},
		RequestedSessionID: "sess-2",
		RequestedAgentName: "ops-agent",
	})

	if entry.MessageID != "req-1" {
		t.Fatalf("expected message id req-1, got %q", entry.MessageID)
	}
	if entry.Hint.TitleHint != "Webhook ticket" {
		t.Fatalf("expected title hint Webhook ticket, got %q", entry.Hint.TitleHint)
	}
	if entry.Scope.EntryPoint != "webhook" {
		t.Fatalf("expected webhook entry point, got %q", entry.Scope.EntryPoint)
	}
	if entry.Scope.ConversationID != "ticket-42" {
		t.Fatalf("expected conversation ticket-42, got %q", entry.Scope.ConversationID)
	}
	if entry.Hint.RequestedSessionID != "sess-2" {
		t.Fatalf("expected requested session sess-2, got %q", entry.Hint.RequestedSessionID)
	}
	if entry.Hint.RequestedAgentName != "ops-agent" {
		t.Fatalf("expected requested agent ops-agent, got %q", entry.Hint.RequestedAgentName)
	}
}
