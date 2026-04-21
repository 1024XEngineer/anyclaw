package state

import (
	"path/filepath"
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

func TestCreateWithOptionsPersistsParticipantsAndGroupFields(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:        "group session",
		AgentName:    "main",
		Participants: []string{"worker-a", "main", "worker-b"},
		Org:          "org-1",
		Project:      "project-1",
		Workspace:    "workspace-1",
		GroupKey:     "group-1",
		IsGroup:      true,
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	if got, want := session.Participants, []string{"main", "worker-a", "worker-b"}; len(got) != len(want) {
		t.Fatalf("expected participants %#v, got %#v", want, got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("expected participants %#v, got %#v", want, got)
			}
		}
	}
	if session.GroupKey != "group-1" {
		t.Fatalf("expected group key to persist, got %q", session.GroupKey)
	}
	if !session.IsGroup {
		t.Fatal("expected session to remain grouped")
	}
}

func TestNewStorePreservesSessionParticipantsAndGroupFields(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)

	session, err := manager.CreateWithOptions(SessionCreateOptions{
		Title:        "persistent group session",
		AgentName:    "main",
		Participants: []string{"worker-a", "worker-b"},
		Org:          "org-1",
		Project:      "project-1",
		Workspace:    "workspace-1",
		GroupKey:     "group-1",
		IsGroup:      true,
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	reloaded, err := NewStore(filepath.Dir(store.path))
	if err != nil {
		t.Fatalf("NewStore reload: %v", err)
	}

	stored, ok := reloaded.GetSession(session.ID)
	if !ok || stored == nil {
		t.Fatal("expected session to reload")
	}
	if got, want := stored.Participants, []string{"main", "worker-a", "worker-b"}; len(got) != len(want) {
		t.Fatalf("expected participants %#v, got %#v", want, got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("expected participants %#v, got %#v", want, got)
			}
		}
	}
	if stored.GroupKey != "group-1" {
		t.Fatalf("expected group key to survive reload, got %q", stored.GroupKey)
	}
	if !stored.IsGroup {
		t.Fatal("expected grouped session after reload")
	}
}
