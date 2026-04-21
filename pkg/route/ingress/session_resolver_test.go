package ingress

import (
	"strings"
	"testing"
)

type stubSessionStore struct {
	sessionsByID       map[string]SessionSnapshot
	sessionsByRouteKey map[string]SessionSnapshot
	createCalls        []SessionCreateOptions
}

func (s stubSessionStore) GetSession(sessionID string) (SessionSnapshot, bool, error) {
	session, ok := s.sessionsByID[sessionID]
	return session, ok, nil
}

func (s stubSessionStore) FindByConversationKey(conversationKey string) (SessionSnapshot, bool, error) {
	session, ok := s.sessionsByRouteKey[conversationKey]
	return session, ok, nil
}

func (s stubSessionStore) BindConversationKey(sessionID string, conversationKey string) (SessionSnapshot, error) {
	session, ok := s.sessionsByID[sessionID]
	if !ok {
		return SessionSnapshot{}, nil
	}
	session.ConversationKey = conversationKey
	return session, nil
}

func (s *stubSessionStore) Create(opts SessionCreateOptions) (SessionSnapshot, error) {
	s.createCalls = append(s.createCalls, opts)
	snapshot := SessionSnapshot{
		ID:              "created-1",
		AgentName:       opts.AgentName,
		ConversationKey: opts.ConversationKey,
		SessionMode:     opts.SessionMode,
		QueueMode:       firstNonEmpty(opts.QueueMode, "fifo"),
		ReplyBack:       opts.ReplyBack,
		ReplyTarget:     opts.ReplyTarget,
		ThreadID:        opts.ThreadID,
		TransportMeta:   cloneStringMap(opts.TransportMeta),
	}
	if s.sessionsByID == nil {
		s.sessionsByID = map[string]SessionSnapshot{}
	}
	s.sessionsByID[snapshot.ID] = snapshot
	if key := strings.TrimSpace(opts.ConversationKey); key != "" {
		if s.sessionsByRouteKey == nil {
			s.sessionsByRouteKey = map[string]SessionSnapshot{}
		}
		s.sessionsByRouteKey[key] = snapshot
	}
	return snapshot, nil
}

func TestSessionResolverPrefersRequestedSessionIDAndBindsConversationKey(t *testing.T) {
	store := &stubSessionStore{
		sessionsByID: map[string]SessionSnapshot{
			"sess-1": {
				ID:        "sess-1",
				AgentName: "AnyClaw",
			},
		},
	}
	resolver := SessionResolver{Sessions: store}

	resolution, snapshot, resolvedAgent, err := resolver.Resolve(MainRouteRequest{
		Hint: RouteHint{RequestedSessionID: "sess-1"},
		Scope: MessageScope{
			ChannelID:      "telegram",
			ConversationID: "chat-1",
		},
	}, RouteDecision{
		RouteKey:    "telegram:chat-1",
		SessionMode: "per-chat",
	}, AgentResolution{
		AgentName: "AnyClaw",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolution.SessionID != "sess-1" {
		t.Fatalf("expected requested session id sess-1, got %q", resolution.SessionID)
	}
	if resolution.MatchedBy != "explicit_session" {
		t.Fatalf("expected explicit_session match, got %q", resolution.MatchedBy)
	}
	if snapshot.ConversationKey != "telegram:chat-1" {
		t.Fatalf("expected conversation key to be bound, got %#v", snapshot)
	}
	if resolvedAgent.AgentName != "AnyClaw" {
		t.Fatalf("expected resolved agent AnyClaw, got %#v", resolvedAgent)
	}
}

func TestSessionResolverReusesConversationKey(t *testing.T) {
	store := &stubSessionStore{
		sessionsByRouteKey: map[string]SessionSnapshot{
			"telegram:chat-9": {
				ID:              "sess-9",
				ConversationKey: "telegram:chat-9",
				AgentName:       "AnyClaw",
			},
		},
	}
	resolver := SessionResolver{
		Sessions: store,
	}

	resolution, snapshot, resolvedAgent, err := resolver.Resolve(MainRouteRequest{}, RouteDecision{
		RouteKey:    "telegram:chat-9",
		SessionMode: "per-chat",
		TitleHint:   "Telegram chat-9",
	}, AgentResolution{
		AgentName: "AnyClaw",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolution.SessionID != "sess-9" {
		t.Fatalf("expected reused session id sess-9, got %q", resolution.SessionID)
	}
	if resolution.MatchedBy != "conversation_key" {
		t.Fatalf("expected conversation_key match, got %q", resolution.MatchedBy)
	}
	if resolution.NeedsCreate {
		t.Fatal("expected conversation-key reuse not to require create")
	}
	if snapshot.AgentName != "AnyClaw" {
		t.Fatalf("expected snapshot agent AnyClaw, got %q", snapshot.AgentName)
	}
	if resolvedAgent.MatchedBy != "conversation_key" {
		t.Fatalf("expected resolved agent to be tagged by conversation_key, got %q", resolvedAgent.MatchedBy)
	}
}

func TestSessionResolverCreatesSessionWhenStoreSupportsCreate(t *testing.T) {
	store := &stubSessionStore{}
	resolver := SessionResolver{
		Sessions: store,
	}

	resolution, snapshot, resolvedAgent, err := resolver.Resolve(MainRouteRequest{
		Text: "create a new session",
		Actor: MessageActor{
			UserID:      "user-7",
			DisplayName: "Alice",
		},
		Scope: MessageScope{
			ChannelID:      "telegram",
			ConversationID: "chat-77",
		},
		DeliveryHint: DeliveryHint{
			ReplyTo: "chat-77",
			Metadata: map[string]string{
				"chat_id": "chat-77",
			},
		},
	}, RouteDecision{
		RouteKey:    "telegram:chat-77",
		SessionMode: "per-chat",
		QueueMode:   "fifo",
		ReplyBack:   true,
		TitleHint:   "Telegram chat-77",
	}, AgentResolution{
		AgentName: "AnyClaw",
		MatchedBy: "default-main",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolution.SessionID != "created-1" {
		t.Fatalf("expected created session id created-1, got %q", resolution.SessionID)
	}
	if !resolution.Created {
		t.Fatal("expected created session to be marked as created")
	}
	if resolution.NeedsCreate {
		t.Fatal("expected created session not to require gateway fallback create")
	}
	if snapshot.ReplyTarget != "chat-77" {
		t.Fatalf("expected snapshot reply target chat-77, got %q", snapshot.ReplyTarget)
	}
	if resolvedAgent.MatchedBy != "created" {
		t.Fatalf("expected resolved agent matched by created, got %q", resolvedAgent.MatchedBy)
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("expected one create call, got %d", len(store.createCalls))
	}
	if store.createCalls[0].ConversationKey != "telegram:chat-77" {
		t.Fatalf("expected create conversation key telegram:chat-77, got %#v", store.createCalls[0])
	}
}
