package ingress

import "strings"

// DeliveryResolver implements M4 for outbound delivery target selection.
type DeliveryResolver struct{}

// Resolve decides where the response should be delivered after execution.
func (r DeliveryResolver) Resolve(request MainRouteRequest, session SessionSnapshot) DeliveryTarget {
	transportMeta := cloneStringMap(session.TransportMeta)
	if len(transportMeta) == 0 {
		transportMeta = cloneStringMap(request.DeliveryHint.Metadata)
	}
	if len(transportMeta) == 0 {
		transportMeta = cloneStringMap(request.Scope.Metadata)
	}
	if transportMeta == nil {
		transportMeta = map[string]string{}
	}

	channelID := strings.TrimSpace(firstNonEmpty(request.DeliveryHint.ChannelID, request.Scope.ChannelID))
	conversationID := strings.TrimSpace(firstNonEmpty(
		session.ReplyTarget,
		request.DeliveryHint.ConversationID,
		request.Scope.ConversationID,
		transportMeta["chat_id"],
		transportMeta["conversation_id"],
	))
	replyTo := strings.TrimSpace(firstNonEmpty(
		session.ReplyTarget,
		request.DeliveryHint.ReplyTo,
		transportMeta["reply_to"],
		transportMeta["reply_target"],
	))
	threadID := strings.TrimSpace(firstNonEmpty(
		session.ThreadID,
		request.DeliveryHint.ThreadID,
		request.Scope.ThreadID,
		transportMeta["thread_id"],
	))

	if channelID != "" {
		transportMeta["channel_id"] = channelID
	}
	if conversationID != "" {
		if strings.TrimSpace(transportMeta["chat_id"]) == "" {
			transportMeta["chat_id"] = conversationID
		}
		if strings.TrimSpace(transportMeta["conversation_id"]) == "" {
			transportMeta["conversation_id"] = conversationID
		}
		if replyTo == "" {
			replyTo = conversationID
		}
	}
	if replyTo != "" {
		transportMeta["reply_target"] = replyTo
	}
	if threadID != "" {
		transportMeta["thread_id"] = threadID
	}
	if session.ConversationKey != "" && strings.TrimSpace(transportMeta["conversation_key"]) == "" {
		transportMeta["conversation_key"] = session.ConversationKey
	}

	return DeliveryTarget{
		ChannelID:      channelID,
		ConversationID: conversationID,
		ReplyTo:        replyTo,
		ThreadID:       threadID,
		TransportMeta:  transportMeta,
	}
}
