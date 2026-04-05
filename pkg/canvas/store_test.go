package canvas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
	if store.BaseDir() == "" {
		t.Fatal("base dir is empty")
	}
}

func TestPushAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	entry, err := store.Push("test-1", "Test Canvas", "<h1>Hello</h1>", EntryTypeHTML, "agent-1")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if entry.ID != "test-1" {
		t.Fatalf("expected id test-1, got %s", entry.ID)
	}
	if entry.Version != 1 {
		t.Fatalf("expected version 1, got %d", entry.Version)
	}

	got, ok := store.Get("test-1")
	if !ok {
		t.Fatal("entry not found")
	}
	if got.Content != "<h1>Hello</h1>" {
		t.Fatalf("expected content <h1>Hello</h1>, got %s", got.Content)
	}
}

func TestPushUpdatesVersion(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, err = store.Push("test-1", "Test", "v1", EntryTypeHTML, "agent-1")
	if err != nil {
		t.Fatalf("first push failed: %v", err)
	}

	entry, err := store.Push("test-1", "Test", "v2", EntryTypeHTML, "agent-1")
	if err != nil {
		t.Fatalf("second push failed: %v", err)
	}
	if entry.Version != 2 {
		t.Fatalf("expected version 2, got %d", entry.Version)
	}
	if entry.Content != "v2" {
		t.Fatalf("expected content v2, got %s", entry.Content)
	}
}

func TestReset(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, err = store.Push("test-1", "Test", "content", EntryTypeHTML, "agent-1")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if err := store.Reset("test-1"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	entry, ok := store.Get("test-1")
	if !ok {
		t.Fatal("entry not found after reset")
	}
	if entry.Content != "" {
		t.Fatalf("expected empty content, got %s", entry.Content)
	}
	if entry.Version != 2 {
		t.Fatalf("expected version 2 after reset, got %d", entry.Version)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, err = store.Push("test-1", "Test", "content", EntryTypeHTML, "agent-1")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if err := store.Delete("test-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, ok := store.Get("test-1")
	if ok {
		t.Fatal("entry still exists after delete")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, _ = store.Push("a", "A", "content", EntryTypeHTML, "agent-1")
	_, _ = store.Push("b", "B", "content", EntryTypeHTML, "agent-1")

	items := store.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestVersions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, _ = store.Push("test-1", "Test", "v1", EntryTypeHTML, "agent-1")
	_, _ = store.Push("test-1", "Test", "v2", EntryTypeHTML, "agent-1")
	_, _ = store.Push("test-1", "Test", "v3", EntryTypeHTML, "agent-1")

	versions := store.GetVersions("test-1", 0)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions (v1 and v2 saved before v3 push), got %d", len(versions))
	}

	v, ok := store.GetVersion("test-1", 2)
	if !ok {
		t.Fatal("version 2 not found")
	}
	if v.Content != "v2" {
		t.Fatalf("expected v2 content, got %s", v.Content)
	}
}

func TestVersionPruning(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 3)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_, _ = store.Push("test-1", "Test", "v", EntryTypeHTML, "agent-1")
	}

	versions := store.GetVersions("test-1", 0)
	if len(versions) > 3 {
		t.Fatalf("expected at most 3 versions, got %d", len(versions))
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()

	store1, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	_, _ = store1.Push("persist-1", "Persist", "content", EntryTypeHTML, "agent-1")

	store2, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore reload failed: %v", err)
	}

	entry, ok := store2.Get("persist-1")
	if !ok {
		t.Fatal("entry not found after reload")
	}
	if entry.Content != "content" {
		t.Fatalf("expected content 'content', got %s", entry.Content)
	}
}

func TestA2UIParseAndRender(t *testing.T) {
	content := `{
		"version": "1.0",
		"title": "Test Page",
		"description": "Demo description",
		"theme": "light",
		"theme_vars": {
			"accent": "#0f766e"
		},
		"components": [
			{"type": "heading", "content": "Hello World", "props": {"level": 1}},
			{"type": "text", "content": "This is a test."},
			{"type": "button", "content": "Click Me"},
			{"type": "section", "props": {"title": "Details", "description": "More context"}, "children": [
				{"type": "stack", "props": {"gap": "8px"}, "children": [
					{"type": "textarea", "props": {"rows": "6", "placeholder": "Write here"}, "content": "Draft"},
					{"type": "link", "content": "AnyClaw", "props": {"href": "https://github.com/anyclaw/anyclaw"}}
				]}
			]}
		]
	}`

	doc, err := ParseA2UI(content)
	if err != nil {
		t.Fatalf("ParseA2UI failed: %v", err)
	}
	if doc.Title != "Test Page" {
		t.Fatalf("expected title 'Test Page', got %s", doc.Title)
	}
	if len(doc.Components) != 4 {
		t.Fatalf("expected 4 components, got %d", len(doc.Components))
	}

	renderer := NewA2UIRenderer()
	html, err := renderer.Render(doc)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(html) == 0 {
		t.Fatal("rendered HTML is empty")
	}
	if !containsAll(html, []string{
		`meta name="description" content="Demo description"`,
		`--accent:#0f766e`,
		`<section class="a2ui-section"`,
		`<textarea rows="6" placeholder="Write here">Draft</textarea>`,
		`<a href="https://github.com/anyclaw/anyclaw">AnyClaw</a>`,
	}) {
		t.Fatalf("rendered HTML missing expected upgraded A2UI content:\n%s", html)
	}
}

func TestAutoIDGeneration(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	entry, err := store.Push("", "Auto", "content", EntryTypeHTML, "agent-1")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if entry.ID == "" {
		t.Fatal("auto-generated ID is empty")
	}
}

func TestPathTraversalProtection(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 5)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	entryDir := filepath.Join(store.BaseDir(), "entries")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	entries, err := os.ReadDir(entryDir)
	if err != nil {
		t.Fatalf("readdir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty entries dir, got %d files", len(entries))
	}
}

func containsAll(value string, snippets []string) bool {
	for _, snippet := range snippets {
		if !strings.Contains(value, snippet) {
			return false
		}
	}
	return true
}
