package memory

import (
	"strings"
	"testing"
	"time"
)

type fakeMemoryBackend struct {
	entries []MemoryEntry
}

func (f *fakeMemoryBackend) Init() error {
	return nil
}

func (f *fakeMemoryBackend) Add(entry MemoryEntry) error {
	f.entries = append(f.entries, entry)
	return nil
}

func (f *fakeMemoryBackend) Get(id string) (*MemoryEntry, error) {
	for i := range f.entries {
		if f.entries[i].ID == id {
			return &f.entries[i], nil
		}
	}
	return nil, nil
}

func (f *fakeMemoryBackend) Delete(id string) error {
	filtered := f.entries[:0]
	for _, entry := range f.entries {
		if entry.ID != id {
			filtered = append(filtered, entry)
		}
	}
	f.entries = filtered
	return nil
}

func (f *fakeMemoryBackend) List() ([]MemoryEntry, error) {
	return append([]MemoryEntry(nil), f.entries...), nil
}

func (f *fakeMemoryBackend) Search(query string, limit int) ([]MemoryEntry, error) {
	return nil, nil
}

func (f *fakeMemoryBackend) Close() error {
	return nil
}

func TestMemoryServiceAddTypedEntries(t *testing.T) {
	t.Parallel()

	backend := &fakeMemoryBackend{}
	svc := NewMemoryService(backend)

	if err := svc.AddReflection("reflect", map[string]string{"kind": "note"}); err != nil {
		t.Fatalf("AddReflection: %v", err)
	}
	if err := svc.AddFact("fact", nil); err != nil {
		t.Fatalf("AddFact: %v", err)
	}

	if len(backend.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(backend.entries))
	}
	if backend.entries[0].Type != TypeReflection {
		t.Fatalf("expected reflection entry, got %q", backend.entries[0].Type)
	}
	if backend.entries[1].Type != TypeFact {
		t.Fatalf("expected fact entry, got %q", backend.entries[1].Type)
	}
}

func TestMemoryServiceBuildsHistoryMarkdownAndStats(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	backend := &fakeMemoryBackend{
		entries: []MemoryEntry{
			{ID: "c3", Type: TypeConversation, Content: "latest", Timestamp: now},
			{ID: "f1", Type: TypeFact, Content: "fact", Timestamp: now.Add(-time.Minute)},
			{ID: "c2", Type: TypeConversation, Content: "middle", Timestamp: now.Add(-2 * time.Minute)},
			{ID: "c1", Type: TypeConversation, Content: "oldest", Timestamp: now.Add(-3 * time.Minute)},
		},
	}
	svc := NewMemoryService(backend)

	history, err := svc.GetConversationHistory(2)
	if err != nil {
		t.Fatalf("GetConversationHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].ID != "c2" || history[1].ID != "c3" {
		t.Fatalf("expected latest conversations in chronological order, got %#v", history)
	}

	stats, err := svc.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats["total"] != 4 {
		t.Fatalf("expected total 4, got %d", stats["total"])
	}
	if stats[TypeConversation] != 3 {
		t.Fatalf("expected 3 conversations, got %d", stats[TypeConversation])
	}
	if stats[TypeFact] != 1 {
		t.Fatalf("expected 1 fact, got %d", stats[TypeFact])
	}

	markdown, err := svc.FormatAsMarkdown()
	if err != nil {
		t.Fatalf("FormatAsMarkdown: %v", err)
	}
	if !strings.Contains(markdown, "# Memory") {
		t.Fatalf("expected markdown header, got %q", markdown)
	}
	if !strings.Contains(markdown, "latest") || !strings.Contains(markdown, "fact") {
		t.Fatalf("expected markdown content, got %q", markdown)
	}
}
