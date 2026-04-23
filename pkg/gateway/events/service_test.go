package events

import (
	"testing"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestAppendEventStoresAndPublishes(t *testing.T) {
	store := newTestStore(t)
	bus := state.NewEventBus()
	sub := bus.Subscribe(1)
	defer bus.Unsubscribe(sub)

	AppendEvent(store, bus, "session.created", "sess-1", map[string]any{"title": "Hello"})

	events := store.ListEvents(10)
	if len(events) != 1 {
		t.Fatalf("expected one stored event, got %d", len(events))
	}
	if events[0].Type != "session.created" || events[0].SessionID != "sess-1" {
		t.Fatalf("unexpected stored event: %#v", events[0])
	}

	select {
	case published := <-sub:
		if published.Type != "session.created" || published.Payload["title"] != "Hello" {
			t.Fatalf("unexpected published event: %#v", published)
		}
	default:
		t.Fatal("expected event to be published to bus")
	}
}

func TestAppendEventAllowsNilStoreAndBus(t *testing.T) {
	AppendEvent(nil, nil, "gateway.start", "", nil)
}

func TestAppendAuditRecordsUserAndAnonymousActors(t *testing.T) {
	store := newTestStore(t)

	AppendAudit(store, &gatewayauth.User{Name: "alice", Role: "admin"}, "create", "session:sess-1", map[string]any{"ok": true})
	AppendAudit(store, nil, "read", "events", nil)

	audit := store.ListAudit(10)
	if len(audit) != 2 {
		t.Fatalf("expected two audit events, got %d", len(audit))
	}
	if audit[0].Actor != "alice" || audit[0].Role != "admin" || audit[0].Action != "create" {
		t.Fatalf("unexpected user audit event: %#v", audit[0])
	}
	if audit[1].Actor != "anonymous" || audit[1].Role != "" || audit[1].Action != "read" {
		t.Fatalf("unexpected anonymous audit event: %#v", audit[1])
	}

	AppendAudit(nil, &gatewayauth.User{Name: "ignored"}, "noop", "target", nil)
}

func TestServiceFacadeDelegatesEventAuditAndPayload(t *testing.T) {
	store := newTestStore(t)
	bus := state.NewEventBus()
	service := NewService(store, bus)

	service.AppendEvent("gateway.start", "", map[string]any{"port": 8080})
	if events := store.ListEvents(10); len(events) != 1 || events[0].Type != "gateway.start" {
		t.Fatalf("expected service to append event, got %#v", events)
	}

	service.AppendAudit(&gatewayauth.User{Name: "bob", Role: "operator"}, "start", "gateway", nil)
	if audit := store.ListAudit(10); len(audit) != 1 || audit[0].Actor != "bob" {
		t.Fatalf("expected service to append audit, got %#v", audit)
	}

	payload := service.SessionCreatedEventPayload(&state.Session{
		Title:     "Chat",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if payload["title"] != "Chat" || payload["org"] != "org-1" || payload["project"] != "project-1" || payload["workspace"] != "workspace-1" {
		t.Fatalf("unexpected session payload: %#v", payload)
	}
}

func newTestStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}
