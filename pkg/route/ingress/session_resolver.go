package ingress

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// SessionResolver implements M3 for conversation-key reuse and explicit session reuse.
type SessionResolver struct {
	Sessions SessionStore
}

// Resolve decides whether this ingress request should reuse an existing session or create a new one.
func (r SessionResolver) Resolve(request MainRouteRequest, decision RouteDecision, agentResolution AgentResolution) (SessionResolution, SessionSnapshot, AgentResolution, error) {
	sessionKey := strings.TrimSpace(firstNonEmpty(decision.RouteKey, derivedSessionKey(request)))

	if snapshot, matchedBy, ok, err := r.resolveExplicitSession(request.Hint.RequestedSessionID, sessionKey, "explicit_session"); err != nil {
		return SessionResolution{}, SessionSnapshot{}, AgentResolution{}, err
	} else if ok {
		return resolutionFromSnapshot(snapshot, decision, matchedBy), snapshot, agentResolutionFromSnapshot(agentResolution, snapshot, matchedBy), nil
	}

	if snapshot, matchedBy, ok, err := r.resolveExplicitSession(decision.ForcedSessionID, sessionKey, "routed_session"); err != nil {
		return SessionResolution{}, SessionSnapshot{}, AgentResolution{}, err
	} else if ok {
		return resolutionFromSnapshot(snapshot, decision, matchedBy), snapshot, agentResolutionFromSnapshot(agentResolution, snapshot, matchedBy), nil
	}

	if r.Sessions != nil && sessionKey != "" {
		snapshot, ok, err := r.Sessions.FindByConversationKey(sessionKey)
		if err != nil {
			return SessionResolution{}, SessionSnapshot{}, AgentResolution{}, err
		}
		if ok {
			return resolutionFromSnapshot(snapshot, decision, "conversation_key"), snapshot, agentResolutionFromSnapshot(agentResolution, snapshot, "conversation_key"), nil
		}
	}

	if r.Sessions != nil {
		snapshot, err := r.Sessions.Create(buildSessionCreateOptions(request, decision, agentResolution, sessionKey))
		if err != nil {
			return SessionResolution{}, SessionSnapshot{}, AgentResolution{}, err
		}
		return resolutionFromCreatedSnapshot(snapshot, decision), snapshot, agentResolutionFromSnapshot(agentResolution, snapshot, "created"), nil
	}

	snapshot := pendingSnapshot(request, decision, agentResolution, sessionKey)
	return SessionResolution{
		SessionID:   "",
		SessionKey:  sessionKey,
		SessionMode: decision.SessionMode,
		QueueMode:   decision.QueueMode,
		ReplyBack:   decision.ReplyBack,
		TitleHint:   decision.TitleHint,
		MatchedBy:   "created",
		MatchedRule: decision.MatchedRule,
		ThreadID:    firstNonEmpty(decision.ThreadID, request.Scope.ThreadID, request.DeliveryHint.ThreadID),
		NeedsCreate: true,
	}, snapshot, agentResolution, nil
}

func (r SessionResolver) resolveExplicitSession(sessionID string, conversationKey string, matchedBy string) (SessionSnapshot, string, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r.Sessions == nil {
		return SessionSnapshot{}, "", false, nil
	}

	if conversationKey != "" {
		snapshot, err := r.Sessions.BindConversationKey(sessionID, conversationKey)
		if err != nil {
			return SessionSnapshot{}, "", false, err
		}
		return snapshot, matchedBy, true, nil
	}

	snapshot, ok, err := r.Sessions.GetSession(sessionID)
	if err != nil {
		return SessionSnapshot{}, "", false, err
	}
	if !ok {
		return SessionSnapshot{}, "", false, fmt.Errorf("session not found: %s", sessionID)
	}
	return snapshot, matchedBy, true, nil
}

func resolutionFromSnapshot(snapshot SessionSnapshot, decision RouteDecision, matchedBy string) SessionResolution {
	sessionKey := strings.TrimSpace(decision.RouteKey)
	if sessionKey == "" {
		sessionKey = strings.TrimSpace(snapshot.ConversationKey)
	}
	return SessionResolution{
		SessionID:   strings.TrimSpace(snapshot.ID),
		SessionKey:  sessionKey,
		SessionMode: firstNonEmpty(snapshot.SessionMode, decision.SessionMode),
		QueueMode:   firstNonEmpty(snapshot.QueueMode, decision.QueueMode),
		ReplyBack:   snapshot.ReplyBack || decision.ReplyBack,
		TitleHint:   decision.TitleHint,
		MatchedBy:   matchedBy,
		MatchedRule: decision.MatchedRule,
		ThreadID:    firstNonEmpty(snapshot.ThreadID, decision.ThreadID),
		Created:     false,
	}
}

func resolutionFromCreatedSnapshot(snapshot SessionSnapshot, decision RouteDecision) SessionResolution {
	resolution := resolutionFromSnapshot(snapshot, decision, "created")
	resolution.Created = true
	return resolution
}

func agentResolutionFromSnapshot(base AgentResolution, snapshot SessionSnapshot, matchedBy string) AgentResolution {
	resolved := base
	if agentName := strings.TrimSpace(snapshot.AgentName); agentName != "" {
		resolved.AgentName = agentName
	}
	if matchedBy != "" {
		resolved.MatchedBy = matchedBy
	}
	return resolved
}

func pendingSnapshot(request MainRouteRequest, decision RouteDecision, agentResolution AgentResolution, sessionKey string) SessionSnapshot {
	transportMeta := cloneStringMap(request.DeliveryHint.Metadata)
	if len(transportMeta) == 0 {
		transportMeta = cloneStringMap(request.Scope.Metadata)
	}
	return SessionSnapshot{
		AgentName:       strings.TrimSpace(agentResolution.AgentName),
		ConversationKey: strings.TrimSpace(sessionKey),
		SessionMode:     strings.TrimSpace(decision.SessionMode),
		QueueMode:       strings.TrimSpace(decision.QueueMode),
		ReplyBack:       decision.ReplyBack,
		ReplyTarget:     strings.TrimSpace(firstNonEmpty(request.DeliveryHint.ReplyTo, request.Scope.ConversationID)),
		ThreadID:        strings.TrimSpace(firstNonEmpty(decision.ThreadID, request.DeliveryHint.ThreadID, request.Scope.ThreadID)),
		TransportMeta:   transportMeta,
	}
}

func buildSessionCreateOptions(request MainRouteRequest, decision RouteDecision, agentResolution AgentResolution, sessionKey string) SessionCreateOptions {
	replyTarget := strings.TrimSpace(firstNonEmpty(
		request.DeliveryHint.ReplyTo,
		request.Scope.ConversationID,
		request.DeliveryHint.ConversationID,
	))
	threadID := strings.TrimSpace(firstNonEmpty(decision.ThreadID, request.DeliveryHint.ThreadID, request.Scope.ThreadID))
	transportMeta := cloneStringMap(request.DeliveryHint.Metadata)
	if len(transportMeta) == 0 {
		transportMeta = cloneStringMap(request.Scope.Metadata)
	}
	return SessionCreateOptions{
		Title:           sessionTitle(request, decision),
		AgentName:       strings.TrimSpace(agentResolution.AgentName),
		SessionMode:     strings.TrimSpace(firstNonEmpty(decision.SessionMode, "channel-dm")),
		QueueMode:       strings.TrimSpace(decision.QueueMode),
		ReplyBack:       decision.ReplyBack,
		SourceChannel:   strings.TrimSpace(request.Scope.ChannelID),
		SourceID:        strings.TrimSpace(firstNonEmpty(request.Actor.UserID, replyTarget, request.Hint.RequestedSessionID)),
		UserID:          strings.TrimSpace(request.Actor.UserID),
		UserName:        strings.TrimSpace(request.Actor.DisplayName),
		ReplyTarget:     replyTarget,
		ThreadID:        threadID,
		ConversationKey: strings.TrimSpace(sessionKey),
		TransportMeta:   transportMeta,
		IsGroup:         request.Scope.IsGroup,
		GroupKey:        strings.TrimSpace(request.Scope.GroupID),
	}
}

func sessionTitle(request MainRouteRequest, decision RouteDecision) string {
	if title := strings.TrimSpace(request.TitleHint); title != "" {
		return title
	}
	if title := strings.TrimSpace(decision.TitleHint); title != "" {
		return title
	}
	source := strings.TrimSpace(firstNonEmpty(request.Scope.ChannelID, "channel"))
	return cases.Title(language.English).String(source) + " session"
}

func derivedSessionKey(request MainRouteRequest) string {
	channelID := strings.TrimSpace(firstNonEmpty(request.Scope.ChannelID, "channel"))
	conversationID := strings.TrimSpace(firstNonEmpty(
		request.DeliveryHint.ReplyTo,
		request.Scope.ConversationID,
		request.DeliveryHint.ConversationID,
		request.Actor.UserID,
	))
	if conversationID == "" {
		return ""
	}
	sessionKey := channelID + ":" + conversationID
	if threadID := strings.TrimSpace(firstNonEmpty(request.Scope.ThreadID, request.DeliveryHint.ThreadID)); threadID != "" {
		sessionKey += ":thread:" + threadID
	}
	return sessionKey
}
