package state

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestShortenTitleTrimsWhitespaceAndPreservesUnicode(t *testing.T) {
	input := "  你好世界你好世界你好世界你好世界你好世界你好世界你好世界你好世界你好世界  "

	title := shortenTitle(input)

	if !utf8.ValidString(title) {
		t.Fatalf("expected valid utf-8 title, got %q", title)
	}
	if strings.HasPrefix(title, " ") || strings.HasSuffix(title, " ") {
		t.Fatalf("expected title to be trimmed, got %q", title)
	}
	if got := len([]rune(title)); got != 36 {
		t.Fatalf("expected full unicode title to be preserved when under limit, got %d runes", got)
	}
}

func TestShortenTitleTruncatesByRune(t *testing.T) {
	input := strings.Repeat("界", 60)

	title := shortenTitle(input)

	if !utf8.ValidString(title) {
		t.Fatalf("expected valid utf-8 title after truncation, got %q", title)
	}
	if got := len([]rune(title)); got != 48 {
		t.Fatalf("expected title to truncate to 48 runes, got %d", got)
	}
}

func TestEnqueueTurnUpdatesLastActiveAt(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)
	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	manager.nowFunc = func() time.Time { return base }

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:     "queue session",
		AgentName: "main",
		Org:       "org-1",
		Project:   "project-1",
		Workspace: "workspace-1",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	queuedAt := base.Add(2 * time.Minute)
	manager.nowFunc = func() time.Time { return queuedAt }
	session.LastActiveAt = base.Add(-10 * time.Minute)
	session.UpdatedAt = base.Add(-10 * time.Minute)
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	updated, err := manager.EnqueueTurn(session.ID)
	if err != nil {
		t.Fatalf("EnqueueTurn: %v", err)
	}

	if updated.LastActiveAt != queuedAt {
		t.Fatalf("expected last_active_at %s, got %s", queuedAt, updated.LastActiveAt)
	}
	if updated.UpdatedAt != queuedAt {
		t.Fatalf("expected updated_at %s, got %s", queuedAt, updated.UpdatedAt)
	}
	if updated.Presence != "queued" {
		t.Fatalf("expected queued presence, got %q", updated.Presence)
	}
}
