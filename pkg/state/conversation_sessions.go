package state

import (
	"fmt"
	"strings"
)

type ConversationSessionResolveOptions struct {
	SessionID       string
	RoutedSessionID string
	ConversationKey string
	CreateOptions   SessionCreateOptions
}

type ConversationSessionResolution struct {
	Session *Session
	Created bool
	Source  string
}

func (m *SessionManager) ResolveConversationSession(opts ConversationSessionResolveOptions) (*ConversationSessionResolution, error) {
	if m == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}

	conversationKey := strings.TrimSpace(opts.ConversationKey)

	if existingID := strings.TrimSpace(opts.SessionID); existingID != "" {
		session, err := m.resolvePinnedConversationSession(existingID, conversationKey)
		if err != nil {
			return nil, err
		}
		return &ConversationSessionResolution{
			Session: session,
			Source:  "explicit_session",
		}, nil
	}

	if routedID := strings.TrimSpace(opts.RoutedSessionID); routedID != "" {
		session, err := m.resolvePinnedConversationSession(routedID, conversationKey)
		if err != nil {
			return nil, err
		}
		return &ConversationSessionResolution{
			Session: session,
			Source:  "routed_session",
		}, nil
	}

	if conversationKey != "" {
		if session, ok := m.FindByConversationKey(conversationKey); ok && session != nil {
			return &ConversationSessionResolution{
				Session: session,
				Source:  "conversation_key",
			}, nil
		}
	}

	createOpts := opts.CreateOptions
	if strings.TrimSpace(createOpts.ConversationKey) == "" {
		createOpts.ConversationKey = conversationKey
	}
	session, err := m.CreateWithOptions(createOpts)
	if err != nil {
		return nil, err
	}
	return &ConversationSessionResolution{
		Session: session,
		Created: true,
		Source:  "created",
	}, nil
}

func (m *SessionManager) resolvePinnedConversationSession(sessionID string, conversationKey string) (*Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id required")
	}

	if strings.TrimSpace(conversationKey) != "" {
		return m.BindConversationKey(sessionID, conversationKey)
	}

	session, ok := m.Get(sessionID)
	if !ok || session == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return session, nil
}
