package ingress

import (
	"fmt"
	"strings"
)

// IngressRouteProjector implements M1 by normalizing one ingress entry.
type IngressRouteProjector struct{}

// Project converts one gateway-trusted ingress entry into the route layer contract.
func (p IngressRouteProjector) Project(entry IngressRoutingEntry) (MainRouteRequest, error) {
	channelID := firstNonEmpty(entry.Scope.ChannelID, entry.Delivery.ChannelID)
	entryPoint := strings.TrimSpace(entry.Scope.EntryPoint)
	if channelID == "" && entryPoint == "" {
		return MainRouteRequest{}, fmt.Errorf("ingress routing entry requires an entry point or channel id")
	}
	if entryPoint == "" {
		entryPoint = "channel"
	}

	scopeMetadata := cloneStringMap(entry.Scope.Metadata)
	deliveryMetadata := cloneStringMap(entry.Delivery.Metadata)
	conversationID := firstNonEmpty(
		entry.Scope.ConversationID,
		entry.Delivery.ConversationID,
		entry.Hint.RequestedSessionID,
		scopeMetadata["conversation_id"],
		scopeMetadata["chat_id"],
	)
	threadID := firstNonEmpty(
		entry.Scope.ThreadID,
		entry.Delivery.ThreadID,
		scopeMetadata["thread_id"],
	)

	scope := MessageScope{
		EntryPoint:     entryPoint,
		ChannelID:      channelID,
		ConversationID: conversationID,
		ThreadID:       threadID,
		GroupID:        firstNonEmpty(entry.Scope.GroupID, scopeMetadata["group_id"], scopeMetadata["guild_id"]),
		IsGroup:        entry.Scope.IsGroup,
		Metadata:       scopeMetadata,
	}
	delivery := DeliveryHint{
		ChannelID:      firstNonEmpty(entry.Delivery.ChannelID, scope.ChannelID),
		ConversationID: firstNonEmpty(entry.Delivery.ConversationID, scope.ConversationID),
		ReplyTo:        strings.TrimSpace(entry.Delivery.ReplyTo),
		ThreadID:       firstNonEmpty(entry.Delivery.ThreadID, scope.ThreadID),
		Metadata:       deliveryMetadata,
	}
	if len(delivery.Metadata) == 0 {
		delivery.Metadata = cloneStringMap(scope.Metadata)
	}

	return MainRouteRequest{
		MessageID: firstNonEmpty(entry.MessageID, scopeMetadata["message_id"]),
		Text:      entry.Text,
		Actor: MessageActor{
			UserID: firstNonEmpty(entry.Actor.UserID, scopeMetadata["user_id"]),
			DisplayName: firstNonEmpty(
				entry.Actor.DisplayName,
				scopeMetadata["user_name"],
				scopeMetadata["username"],
				scopeMetadata["display_name"],
			),
		},
		Scope:        scope,
		DeliveryHint: delivery,
		Hint: RouteHint{
			RequestedAgentName: strings.TrimSpace(entry.Hint.RequestedAgentName),
			RequestedSessionID: strings.TrimSpace(entry.Hint.RequestedSessionID),
			TitleHint:          strings.TrimSpace(entry.Hint.TitleHint),
		},
		ReceivedAt: entry.ReceivedAt,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		out[trimmedKey] = trimmedValue
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
