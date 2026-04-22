package ingress

import (
	"context"
	"fmt"
	"strings"
	"time"

	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
)

// MessageInput is the gateway-facing handoff contract used before routing.
type MessageInput struct {
	EntryPoint         string
	Source             string
	SessionID          string
	Message            string
	TitleHint          string
	ConversationID     string
	ThreadID           string
	GroupID            string
	IsGroup            bool
	ActorUserID        string
	ActorDisplayName   string
	RequestedAgentName string
	RequestedSessionID string
	Meta               map[string]string
}

// ChannelInput is the gateway-facing input contract for one channel ingress message.
type ChannelInput struct {
	Source    string
	SessionID string
	Message   string
	Meta      map[string]string
}

// SignedInput is the gateway-facing input contract for one signed webhook message.
type SignedInput struct {
	SessionID string
	Title     string
	Message   string
	Meta      map[string]string
}

// Service is the thin gateway-to-route handoff for business ingress.
type Service struct {
	Routes    *routeingress.Service
	CloneMeta func(map[string]string) map[string]string
}

// PrepareNormalizedEntry converts a governance-normalized request into one trusted route-layer entry.
func (s Service) PrepareNormalizedEntry(request gatewaygovernance.NormalizedRequest) routeingress.IngressRoutingEntry {
	meta := request.RouteContext.Metadata
	if s.CloneMeta != nil {
		meta = s.CloneMeta(request.RouteContext.Metadata)
	}
	return routeingress.IngressRoutingEntry{
		MessageID: request.RequestID,
		Text:      strings.TrimSpace(request.ContentText),
		Actor: routeingress.MessageActor{
			UserID:      strings.TrimSpace(request.Actor.UserID),
			DisplayName: strings.TrimSpace(request.Actor.DisplayName),
		},
		Scope: routeingress.MessageScope{
			EntryPoint:     strings.TrimSpace(request.RouteContext.EntryPoint),
			ChannelID:      strings.TrimSpace(request.RouteContext.ChannelID),
			ConversationID: strings.TrimSpace(request.RouteContext.ConversationID),
			ThreadID:       strings.TrimSpace(request.RouteContext.ThreadID),
			GroupID:        strings.TrimSpace(request.RouteContext.GroupID),
			IsGroup:        request.RouteContext.IsGroup,
			Metadata:       meta,
		},
		Delivery: routeingress.DeliveryHint{
			ChannelID:      strings.TrimSpace(request.RouteContext.ChannelID),
			ConversationID: strings.TrimSpace(request.RouteContext.ConversationID),
			ReplyTo:        strings.TrimSpace(request.RouteContext.Delivery.ReplyTarget),
			ThreadID:       strings.TrimSpace(request.RouteContext.Delivery.ThreadID),
			Metadata:       meta,
		},
		Hint: routeingress.RouteHint{
			RequestedAgentName: strings.TrimSpace(request.RequestedAgentName),
			RequestedSessionID: strings.TrimSpace(request.RequestedSessionID),
			TitleHint:          strings.TrimSpace(request.TitleHint),
		},
		ReceivedAt: time.Now().UTC(),
	}
}

// PrepareMessageEntry converts gateway facts into one trusted route-layer entry.
func (s Service) PrepareMessageEntry(input MessageInput) routeingress.IngressRoutingEntry {
	meta := input.Meta
	if s.CloneMeta != nil {
		meta = s.CloneMeta(input.Meta)
	}

	entryPoint := strings.TrimSpace(input.EntryPoint)
	if entryPoint == "" {
		entryPoint = "channel"
	}
	channelID := firstNonEmpty(strings.TrimSpace(input.Source), strings.TrimSpace(meta["channel"]), strings.TrimSpace(meta["channel_id"]))
	conversationID := firstNonEmpty(
		strings.TrimSpace(input.ConversationID),
		strings.TrimSpace(meta["reply_target"]),
		strings.TrimSpace(input.SessionID),
		strings.TrimSpace(meta["conversation_id"]),
		strings.TrimSpace(meta["chat_id"]),
	)
	threadID := firstNonEmpty(strings.TrimSpace(input.ThreadID), strings.TrimSpace(meta["thread_id"]))
	groupID := firstNonEmpty(strings.TrimSpace(input.GroupID), strings.TrimSpace(meta["group_id"]), strings.TrimSpace(meta["guild_id"]))
	actorUserID := firstNonEmpty(strings.TrimSpace(input.ActorUserID), strings.TrimSpace(meta["user_id"]))
	actorDisplayName := firstNonEmpty(
		strings.TrimSpace(input.ActorDisplayName),
		strings.TrimSpace(meta["username"]),
		strings.TrimSpace(meta["user_name"]),
		strings.TrimSpace(meta["display_name"]),
	)
	requestedSessionID := firstNonEmpty(strings.TrimSpace(input.RequestedSessionID), strings.TrimSpace(input.SessionID))
	requestedAgentName := firstNonEmpty(
		strings.TrimSpace(input.RequestedAgentName),
		strings.TrimSpace(meta["agent_name"]),
		strings.TrimSpace(meta["assistant_name"]),
		strings.TrimSpace(meta["agent"]),
		strings.TrimSpace(meta["assistant"]),
	)

	return routeingress.IngressRoutingEntry{
		Text: strings.TrimSpace(input.Message),
		Actor: routeingress.MessageActor{
			UserID:      actorUserID,
			DisplayName: actorDisplayName,
		},
		Scope: routeingress.MessageScope{
			EntryPoint:     entryPoint,
			ChannelID:      channelID,
			ConversationID: conversationID,
			ThreadID:       threadID,
			GroupID:        groupID,
			IsGroup:        input.IsGroup || strings.EqualFold(strings.TrimSpace(meta["is_group"]), "true"),
			Metadata:       meta,
		},
		Delivery: routeingress.DeliveryHint{
			ChannelID:      channelID,
			ConversationID: conversationID,
			ReplyTo:        strings.TrimSpace(meta["reply_target"]),
			ThreadID:       threadID,
			Metadata:       meta,
		},
		Hint: routeingress.RouteHint{
			RequestedAgentName: requestedAgentName,
			RequestedSessionID: requestedSessionID,
			TitleHint:          strings.TrimSpace(firstNonEmpty(input.TitleHint, meta["title_hint"], meta["title"])),
		},
		ReceivedAt: time.Now().UTC(),
	}
}

// PrepareChannelEntry converts gateway channel facts into a trusted route-layer entry.
func (s Service) PrepareChannelEntry(input ChannelInput) routeingress.IngressRoutingEntry {
	return s.PrepareMessageEntry(MessageInput{
		EntryPoint: "channel",
		Source:     input.Source,
		SessionID:  input.SessionID,
		Message:    input.Message,
		Meta:       input.Meta,
	})
}

// PrepareSignedEntry converts a signed webhook message into one trusted route-layer entry.
func (s Service) PrepareSignedEntry(input SignedInput) routeingress.IngressRoutingEntry {
	return s.PrepareMessageEntry(MessageInput{
		EntryPoint:         "webhook",
		Source:             "webhook",
		SessionID:          input.SessionID,
		Message:            input.Message,
		TitleHint:          input.Title,
		RequestedSessionID: input.SessionID,
		Meta:               input.Meta,
	})
}

// RouteMessage hands one prepared ingress entry to the route service.
func (s Service) RouteMessage(ctx context.Context, input MessageInput) (routeingress.RouteOutput, error) {
	if s.Routes == nil {
		return routeingress.RouteOutput{}, fmt.Errorf("ingress service not initialized")
	}
	return s.Routes.Route(routeingress.RouteInput{
		Entry: s.PrepareMessageEntry(input),
	})
}

// RouteNormalized hands one governance-normalized request to the route service.
func (s Service) RouteNormalized(ctx context.Context, request gatewaygovernance.NormalizedRequest) (routeingress.RouteOutput, error) {
	if s.Routes == nil {
		return routeingress.RouteOutput{}, fmt.Errorf("ingress service not initialized")
	}
	return s.Routes.Route(routeingress.RouteInput{
		Entry: s.PrepareNormalizedEntry(request),
	})
}

// RouteChannel hands one prepared ingress entry to the route service.
func (s Service) RouteChannel(ctx context.Context, input ChannelInput) (routeingress.RouteOutput, error) {
	return s.RouteMessage(ctx, MessageInput{
		EntryPoint: "channel",
		Source:     input.Source,
		SessionID:  input.SessionID,
		Message:    input.Message,
		Meta:       input.Meta,
	})
}

// RouteSigned hands one prepared signed ingress entry to the route service.
func (s Service) RouteSigned(ctx context.Context, input SignedInput) (routeingress.RouteOutput, error) {
	return s.RouteMessage(ctx, MessageInput{
		EntryPoint:         "webhook",
		Source:             "webhook",
		SessionID:          input.SessionID,
		Message:            input.Message,
		TitleHint:          input.Title,
		RequestedSessionID: input.SessionID,
		Meta:               input.Meta,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
