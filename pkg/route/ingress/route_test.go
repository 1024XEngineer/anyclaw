package ingress

import (
	"context"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
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
}

func TestAgentResolverUsesRequestedAndMainAgentFallback(t *testing.T) {
	resolver := AgentResolver{
		ResolveMainAgentName: func() string {
			return "AnyClaw"
		},
	}

	defaultResolution, decision, err := resolver.Resolve(MainRouteRequest{})
	if err != nil {
		t.Fatalf("Resolve default: %v", err)
	}
	if decision.RouteKey != "" {
		t.Fatalf("expected empty decision without router, got %#v", decision)
	}
	if defaultResolution.AgentName != "AnyClaw" || defaultResolution.MatchedBy != "default-main" {
		t.Fatalf("expected default main agent resolution, got %#v", defaultResolution)
	}

	mainAliasResolution, _, err := resolver.Resolve(MainRouteRequest{
		Hint: RouteHint{RequestedAgentName: "main"},
	})
	if err != nil {
		t.Fatalf("Resolve main alias: %v", err)
	}
	if mainAliasResolution.AgentName != "AnyClaw" || mainAliasResolution.MatchedBy != "requested-main" {
		t.Fatalf("expected requested main agent resolution, got %#v", mainAliasResolution)
	}

	specialistResolution, _, err := resolver.Resolve(MainRouteRequest{
		Hint: RouteHint{RequestedAgentName: "vision-agent"},
	})
	if err != nil {
		t.Fatalf("Resolve specialist: %v", err)
	}
	if specialistResolution.AgentName != "vision-agent" || specialistResolution.MatchedBy != "requested" {
		t.Fatalf("expected requested specialist resolution, got %#v", specialistResolution)
	}
}

func TestServiceRouteRunsM1AndM2(t *testing.T) {
	service := NewService(
		NewRouter(config.RoutingConfig{Mode: "per-chat"}),
		WithMainAgentNameResolver(func() string { return "AnyClaw" }),
	)

	output, err := service.Route(context.Background(), RouteInput{
		Entry: IngressRoutingEntry{
			MessageID: "msg-9",
			Text:      "route this message",
			Scope: MessageScope{
				ChannelID:      "telegram",
				ConversationID: "chat-9",
			},
			Delivery: DeliveryHint{
				ReplyTo: "reply-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}

	if output.Request.Request.MessageID != "msg-9" {
		t.Fatalf("expected message id msg-9, got %q", output.Request.Request.MessageID)
	}
	if output.Request.Request.Scope.ChannelID != "telegram" {
		t.Fatalf("expected telegram channel, got %q", output.Request.Request.Scope.ChannelID)
	}
	if output.Request.Route.Agent.AgentName != "AnyClaw" {
		t.Fatalf("expected AnyClaw agent, got %q", output.Request.Route.Agent.AgentName)
	}
	if output.Request.Route.Agent.MatchedBy != "default-main" {
		t.Fatalf("expected default-main resolution, got %q", output.Request.Route.Agent.MatchedBy)
	}
	if output.Request.Route.Session.SessionKey != "telegram:chat-9" {
		t.Fatalf("expected derived session key telegram:chat-9, got %q", output.Request.Route.Session.SessionKey)
	}
	if !output.Request.Route.Session.NeedsCreate {
		t.Fatal("expected route output without a session store to require create")
	}
	if output.Request.Route.Delivery.ChannelID != "telegram" {
		t.Fatalf("expected delivery channel telegram, got %q", output.Request.Route.Delivery.ChannelID)
	}
	if output.Request.Route.Delivery.ConversationID != "reply-1" {
		t.Fatalf("expected delivery conversation reply-1, got %q", output.Request.Route.Delivery.ConversationID)
	}
	if output.Request.Route.Delivery.ReplyTo != "reply-1" {
		t.Fatalf("expected delivery reply target reply-1, got %q", output.Request.Route.Delivery.ReplyTo)
	}
	if output.Request.Route.Delivery.TransportMeta["reply_target"] != "reply-1" {
		t.Fatalf("expected delivery metadata reply_target reply-1, got %#v", output.Request.Route.Delivery.TransportMeta)
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

func TestServiceRouteRunsM3WithSessionStore(t *testing.T) {
	store := &stubSessionStore{
		sessionsByRouteKey: map[string]SessionSnapshot{
			"telegram:chat-22": {
				ID:              "sess-22",
				ConversationKey: "telegram:chat-22",
				ReplyTarget:     "chat-22",
			},
		},
	}
	service := NewService(
		NewRouter(config.RoutingConfig{Mode: "per-chat"}),
		WithMainAgentNameResolver(func() string { return "AnyClaw" }),
		WithSessionStore(store),
	)

	output, err := service.Route(context.Background(), RouteInput{
		Entry: IngressRoutingEntry{
			Text: "reuse this session",
			Scope: MessageScope{
				ChannelID:      "telegram",
				ConversationID: "chat-22",
			},
		},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}

	if output.Request.Route.Session.SessionID != "sess-22" {
		t.Fatalf("expected routed session id sess-22, got %q", output.Request.Route.Session.SessionID)
	}
	if output.Request.Route.Session.MatchedBy != "conversation_key" {
		t.Fatalf("expected conversation_key session match, got %q", output.Request.Route.Session.MatchedBy)
	}
	if output.Request.Route.Session.NeedsCreate {
		t.Fatal("expected existing session to avoid create")
	}
	if output.Request.Route.Delivery.ConversationID != "chat-22" {
		t.Fatalf("expected delivery conversation chat-22, got %q", output.Request.Route.Delivery.ConversationID)
	}
	if output.Request.Route.Delivery.TransportMeta["conversation_key"] != "telegram:chat-22" {
		t.Fatalf("expected delivery metadata conversation_key telegram:chat-22, got %#v", output.Request.Route.Delivery.TransportMeta)
	}
	if len(store.createCalls) != 0 {
		t.Fatalf("expected reuse path not to create sessions, got %d create calls", len(store.createCalls))
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

func TestDeliveryResolverCopiesTransportFacts(t *testing.T) {
	resolver := DeliveryResolver{}

	target := resolver.Resolve(MainRouteRequest{
		Scope: MessageScope{
			ChannelID:      "discord",
			ConversationID: "room-8",
			ThreadID:       "thread-2",
			Metadata: map[string]string{
				"guild_id": "guild-1",
			},
		},
		DeliveryHint: DeliveryHint{
			ReplyTo: "reply-room-8",
			Metadata: map[string]string{
				"chat_id": "room-8",
			},
		},
	}, SessionSnapshot{
		ConversationKey: "discord:room-8:thread:thread-2",
	})

	if target.ChannelID != "discord" {
		t.Fatalf("expected discord delivery channel, got %q", target.ChannelID)
	}
	if target.ConversationID != "room-8" {
		t.Fatalf("expected room-8 delivery conversation, got %q", target.ConversationID)
	}
	if target.ReplyTo != "reply-room-8" {
		t.Fatalf("expected reply-room-8 delivery reply target, got %q", target.ReplyTo)
	}
	if target.ThreadID != "thread-2" {
		t.Fatalf("expected thread-2 delivery thread, got %q", target.ThreadID)
	}
	if target.TransportMeta["channel_id"] != "discord" || target.TransportMeta["chat_id"] != "room-8" || target.TransportMeta["conversation_key"] != "discord:room-8:thread:thread-2" {
		t.Fatalf("expected delivery metadata to preserve transport facts, got %#v", target.TransportMeta)
	}
}
