package state

import "testing"

func TestResolveConversationSessionCreatesAndReusesConversationKey(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	first, err := manager.ResolveConversationSession(ConversationSessionResolveOptions{
		ConversationKey: "telegram:chat-1",
		CreateOptions: SessionCreateOptions{
			Title:       "Telegram chat-1",
			AgentName:   "MainAgent",
			Org:         "org-1",
			Project:     "project-1",
			Workspace:   "workspace-1",
			SessionMode: "per-chat",
		},
	})
	if err != nil {
		t.Fatalf("first ResolveConversationSession: %v", err)
	}
	if !first.Created {
		t.Fatal("expected first resolution to create a session")
	}
	if first.Session == nil || first.Session.ConversationKey != "telegram:chat-1" {
		t.Fatalf("expected conversation key to be stored, got %#v", first.Session)
	}

	second, err := manager.ResolveConversationSession(ConversationSessionResolveOptions{
		ConversationKey: "telegram:chat-1",
		CreateOptions: SessionCreateOptions{
			Title:       "ignored",
			AgentName:   "MainAgent",
			Org:         "org-1",
			Project:     "project-1",
			Workspace:   "workspace-1",
			SessionMode: "per-chat",
		},
	})
	if err != nil {
		t.Fatalf("second ResolveConversationSession: %v", err)
	}
	if second.Created {
		t.Fatal("expected second resolution to reuse an existing session")
	}
	if second.Source != "conversation_key" {
		t.Fatalf("expected resolution source conversation_key, got %q", second.Source)
	}
	if second.Session == nil || second.Session.ID != first.Session.ID {
		t.Fatalf("expected session %q to be reused, got %#v", first.Session.ID, second.Session)
	}
}

func TestResolveConversationSessionBindsPinnedSessionToConversationKey(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:     "Pinned session",
		AgentName: "MainAgent",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	resolution, err := manager.ResolveConversationSession(ConversationSessionResolveOptions{
		SessionID:       session.ID,
		ConversationKey: "telegram:chat-2",
	})
	if err != nil {
		t.Fatalf("ResolveConversationSession: %v", err)
	}
	if resolution.Created {
		t.Fatal("expected pinned session resolution to reuse the existing session")
	}
	if resolution.Source != "explicit_session" {
		t.Fatalf("expected resolution source explicit_session, got %q", resolution.Source)
	}
	if resolution.Session == nil || resolution.Session.ID != session.ID {
		t.Fatalf("expected pinned session %q, got %#v", session.ID, resolution.Session)
	}
	if resolution.Session.ConversationKey != "telegram:chat-2" {
		t.Fatalf("expected conversation key to be bound, got %q", resolution.Session.ConversationKey)
	}

	stored, ok := manager.FindByConversationKey("telegram:chat-2")
	if !ok || stored == nil {
		t.Fatal("expected conversation key lookup to find the pinned session")
	}
	if stored.ID != session.ID {
		t.Fatalf("expected stored session %q, got %q", session.ID, stored.ID)
	}
}

func TestBindConversationKeyEvictsPreviousSessionBinding(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	first, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:           "First",
		AgentName:       "MainAgent",
		Org:             "org-1",
		Project:         "project-1",
		Workspace:       "workspace-1",
		ConversationKey: "telegram:chat-3",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions(first): %v", err)
	}
	second, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:     "Second",
		AgentName: "MainAgent",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions(second): %v", err)
	}

	updated, err := manager.BindConversationKey(second.ID, "telegram:chat-3")
	if err != nil {
		t.Fatalf("BindConversationKey: %v", err)
	}
	if updated.ConversationKey != "telegram:chat-3" {
		t.Fatalf("expected rebound key on new session, got %q", updated.ConversationKey)
	}

	storedFirst, ok := store.GetSession(first.ID)
	if !ok || storedFirst == nil {
		t.Fatal("expected first session to remain in store")
	}
	if storedFirst.ConversationKey != "" {
		t.Fatalf("expected old binding to be cleared, got %q", storedFirst.ConversationKey)
	}

	found, ok := manager.FindByConversationKey("telegram:chat-3")
	if !ok || found == nil {
		t.Fatal("expected conversation key lookup to succeed")
	}
	if found.ID != second.ID {
		t.Fatalf("expected latest bound session %q, got %q", second.ID, found.ID)
	}
}

func TestEnqueueTurnResetsStaleQueueDepthWhenSessionIsIdle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:     "Stale queue session",
		AgentName: "MainAgent",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}
	session.QueueDepth = 3
	session.Presence = "idle"
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	updatedSession, err := manager.EnqueueTurn(session.ID)
	if err != nil {
		t.Fatalf("EnqueueTurn: %v", err)
	}
	if updatedSession.QueueDepth != 1 {
		t.Fatalf("expected stale queue depth to reset before enqueue, got %d", updatedSession.QueueDepth)
	}
	if updatedSession.Presence != "queued" {
		t.Fatalf("expected queued presence after enqueue, got %q", updatedSession.Presence)
	}
}

func TestCreateWithOptionsPreservesGroupFields(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:        "Group session",
		AgentName:    "MainAgent",
		Participants: []string{"MainAgent", "Reviewer"},
		Org:          "org-1",
		Project:      "project-1",
		Workspace:    "workspace-1",
		GroupKey:     "group-1",
		IsGroup:      true,
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	if len(session.Participants) != 2 {
		t.Fatalf("expected participants to round-trip, got %#v", session.Participants)
	}
	if session.GroupKey != "group-1" {
		t.Fatalf("expected group key to round-trip, got %q", session.GroupKey)
	}
	if !session.IsGroup {
		t.Fatal("expected group flag to round-trip")
	}
}
