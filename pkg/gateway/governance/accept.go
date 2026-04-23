package governance

import (
	"context"
	"fmt"
	"strings"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

// Accept performs governance normalization for one business ingress request.
func (s Service) Accept(ctx context.Context, raw RawRequest) (NormalizedRequest, error) {
	text := strings.TrimSpace(raw.Message)
	if text == "" {
		return NormalizedRequest{}, fmt.Errorf("message is required")
	}

	requestID := strings.TrimSpace(raw.RequestID)
	if requestID == "" {
		requestID = state.UniqueID("req")
	}
	meta := cloneStringMap(raw.Metadata)
	entryPoint := firstNonEmpty(strings.TrimSpace(raw.EntryPoint), strings.TrimSpace(raw.SourceType), "channel")
	sourceType := firstNonEmpty(strings.TrimSpace(raw.SourceType), entryPoint)
	channelID := firstNonEmpty(strings.TrimSpace(raw.ChannelID), strings.TrimSpace(meta["channel"]), strings.TrimSpace(meta["channel_id"]))
	replyTarget := firstNonEmpty(strings.TrimSpace(raw.DeliveryReplyTarget), strings.TrimSpace(meta["reply_target"]))
	conversationID := firstNonEmpty(
		strings.TrimSpace(raw.ConversationID),
		replyTarget,
		strings.TrimSpace(raw.SessionID),
		strings.TrimSpace(meta["conversation_id"]),
		strings.TrimSpace(meta["chat_id"]),
	)
	threadID := firstNonEmpty(strings.TrimSpace(raw.ThreadID), strings.TrimSpace(meta["thread_id"]))
	groupID := firstNonEmpty(strings.TrimSpace(raw.GroupID), strings.TrimSpace(meta["group_id"]), strings.TrimSpace(meta["guild_id"]))
	peerID := firstNonEmpty(strings.TrimSpace(raw.PeerID), conversationID, strings.TrimSpace(raw.ActorUserID))
	peerKind := firstNonEmpty(strings.TrimSpace(raw.PeerKind), derivePeerKind(raw.IsGroup))
	actor := actorFromContext(s.currentUser(ctx), raw)

	return NormalizedRequest{
		RequestID:   requestID,
		SourceType:  sourceType,
		Actor:       actor,
		ContentText: text,
		TitleHint:   strings.TrimSpace(firstNonEmpty(raw.TitleHint, meta["title_hint"], meta["title"])),
		RouteContext: RouteContext{
			EntryPoint:     entryPoint,
			SourceType:     sourceType,
			ChannelID:      channelID,
			ConversationID: conversationID,
			ThreadID:       threadID,
			GroupID:        groupID,
			IsGroup:        raw.IsGroup || strings.EqualFold(strings.TrimSpace(meta["is_group"]), "true"),
			PeerID:         peerID,
			PeerKind:       peerKind,
			Delivery: DeliveryHint{
				ReplyTarget:   replyTarget,
				ThreadID:      threadID,
				TransportMeta: meta,
			},
			Metadata: meta,
		},
		Governance: GovernanceResult{
			Authenticated: actor.Authenticated,
			PermissionSet: cloneStrings(actor.Roles),
			RateLimitKey:  buildRateLimitKey(sourceType, channelID, peerID, actor.UserID),
			RiskLevel:     "normal",
		},
		RequestedAgentName: strings.TrimSpace(raw.RequestedAgentName),
		RequestedSessionID: strings.TrimSpace(firstNonEmpty(raw.RequestedSessionID, raw.SessionID)),
	}, nil
}

func actorFromContext(user *gatewayauth.User, raw RawRequest) ActorRef {
	if user == nil {
		return ActorRef{
			UserID:        strings.TrimSpace(raw.ActorUserID),
			DisplayName:   strings.TrimSpace(raw.ActorDisplayName),
			Authenticated: false,
		}
	}
	return ActorRef{
		UserID:        firstNonEmpty(strings.TrimSpace(raw.ActorUserID), strings.TrimSpace(user.Name)),
		AccountID:     strings.TrimSpace(user.Name),
		DisplayName:   firstNonEmpty(strings.TrimSpace(raw.ActorDisplayName), strings.TrimSpace(user.Name)),
		Roles:         append([]string{strings.TrimSpace(user.Role)}, cloneStrings(user.PermissionOverrides)...),
		Authenticated: true,
	}
}

func buildRateLimitKey(sourceType string, channelID string, peerID string, actorID string) string {
	return firstNonEmpty(
		strings.TrimSpace(sourceType)+":"+strings.TrimSpace(channelID)+":"+strings.TrimSpace(peerID),
		strings.TrimSpace(sourceType)+":"+strings.TrimSpace(actorID),
		strings.TrimSpace(sourceType),
	)
}

func derivePeerKind(isGroup bool) string {
	if isGroup {
		return "group"
	}
	return "direct"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
			cloned[trimmedKey] = strings.TrimSpace(value)
		}
	}
	return cloned
}

func cloneStrings(source []string) []string {
	if len(source) == 0 {
		return nil
	}
	cloned := make([]string, 0, len(source))
	for _, value := range source {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cloned = append(cloned, trimmed)
		}
	}
	return cloned
}
