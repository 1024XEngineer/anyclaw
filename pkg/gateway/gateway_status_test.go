package gateway

import (
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestTypingSessionActiveIgnoresStaleSessions(t *testing.T) {
	now := time.Date(2026, 4, 12, 2, 0, 0, 0, time.UTC)

	if typingSessionActive(nil, now, typingSessionStaleAfter) {
		t.Fatal("nil session should not be active")
	}

	stale := &state.Session{
		Typing:       true,
		LastActiveAt: now.Add(-typingSessionStaleAfter - time.Second),
		UpdatedAt:    now.Add(-typingSessionStaleAfter - time.Second),
	}
	if typingSessionActive(stale, now, typingSessionStaleAfter) {
		t.Fatal("stale typing session should not be treated as active")
	}

	fresh := &state.Session{
		Typing:       true,
		LastActiveAt: now.Add(-5 * time.Second),
		UpdatedAt:    now.Add(-5 * time.Second),
	}
	if !typingSessionActive(fresh, now, typingSessionStaleAfter) {
		t.Fatal("fresh typing session should be treated as active")
	}

	fallback := &state.Session{
		Typing:    true,
		UpdatedAt: now.Add(-3 * time.Second),
	}
	if !typingSessionActive(fallback, now, typingSessionStaleAfter) {
		t.Fatal("updated_at should be used when last_active_at is missing")
	}
}
