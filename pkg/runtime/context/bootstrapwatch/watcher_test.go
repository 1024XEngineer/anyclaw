package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", name, err)
	}
	return path
}

func TestWatcherLoad(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "# Agent Config\nname: test")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
	})

	entry, ok := w.Get(FileAgents)
	if !ok {
		t.Fatal("expected AGENTS.md entry")
	}
	if entry.Content != "# Agent Config\nname: test" {
		t.Errorf("unexpected content: %s", entry.Content)
	}
	if entry.Size == 0 {
		t.Error("expected non-zero size")
	}
}

func TestWatcherAutoDetect(t *testing.T) {
	dir := setupTestDir(t)

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
	})

	_, ok := w.Get(FileSoul)
	if ok {
		t.Error("expected SOUL.md not loaded initially")
	}

	writeFile(t, dir, "SOUL.md", "# Soul\npurpose: help")

	time.Sleep(100 * time.Millisecond)
	w.checkChanges()

	_, ok = w.Get(FileSoul)
	if !ok {
		t.Error("expected SOUL.md detected after creation")
	}
}

func TestWatcherModification(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "initial content")

	var changes []ChangeEvent
	var mu sync.Mutex

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
		OnChange: func(event ChangeEvent) {
			mu.Lock()
			changes = append(changes, event)
			mu.Unlock()
		},
	})

	writeFile(t, dir, "AGENTS.md", "updated content")

	time.Sleep(200 * time.Millisecond)
	w.checkChanges()

	mu.Lock()
	if len(changes) == 0 {
		mu.Unlock()
		t.Fatal("expected change event")
	}
	event := changes[0]
	mu.Unlock()

	if event.Action != ActionModified {
		t.Errorf("expected modified action, got %s", event.Action)
	}
	if event.Type != FileAgents {
		t.Errorf("expected FileAgents type, got %s", event.Type)
	}

	entry, _ := w.Get(FileAgents)
	if entry.Content != "updated content" {
		t.Errorf("expected updated content, got %s", entry.Content)
	}
}

func TestWatcherDeletion(t *testing.T) {
	dir := setupTestDir(t)
	path := writeFile(t, dir, "AGENTS.md", "content")

	var changes []ChangeEvent
	var mu sync.Mutex

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
		OnChange: func(event ChangeEvent) {
			mu.Lock()
			changes = append(changes, event)
			mu.Unlock()
		},
	})

	os.Remove(path)

	time.Sleep(200 * time.Millisecond)
	w.checkChanges()

	mu.Lock()
	if len(changes) == 0 {
		mu.Unlock()
		t.Fatal("expected deletion event")
	}
	event := changes[0]
	mu.Unlock()

	if event.Action != ActionDeleted {
		t.Errorf("expected deleted action, got %s", event.Action)
	}

	_, ok := w.Get(FileAgents)
	if ok {
		t.Error("expected AGENTS.md removed after deletion")
	}
}

func TestWatcherOnChange(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "initial")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
	})

	var called bool
	var mu sync.Mutex
	w.OnChange(func(event ChangeEvent) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	writeFile(t, dir, "AGENTS.md", "changed")

	time.Sleep(200 * time.Millisecond)
	w.checkChanges()

	mu.Lock()
	if !called {
		t.Error("expected OnChange handler called")
	}
	mu.Unlock()
}

func TestWatcherReload(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "v1")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
	})

	writeFile(t, dir, "AGENTS.md", "v2")

	if err := w.Reload(FileAgents); err != nil {
		t.Fatalf("reload: %v", err)
	}

	entry, _ := w.Get(FileAgents)
	if entry.Content != "v2" {
		t.Errorf("expected v2 after reload, got %s", entry.Content)
	}
}

func TestWatcherReloadAll(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "agents v2")
	writeFile(t, dir, "SOUL.md", "soul v2")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents, FileSoul},
	})

	if err := w.ReloadAll(); err != nil {
		t.Fatalf("reload all: %v", err)
	}

	agents, _ := w.Get(FileAgents)
	if agents.Content != "agents v2" {
		t.Errorf("expected agents v2, got %s", agents.Content)
	}

	soul, _ := w.Get(FileSoul)
	if soul.Content != "soul v2" {
		t.Errorf("expected soul v2, got %s", soul.Content)
	}
}

func TestWatcherReloadAllReturnsAllErrors(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "agents")
	writeFile(t, dir, "SOUL.md", "soul")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents, FileSoul},
	})

	w.baseDir = "\x00"

	err := w.ReloadAll()
	if err == nil {
		t.Fatal("expected reload all error")
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("expected joined error, got %T", err)
	}
	if len(joined.Unwrap()) != 2 {
		t.Fatalf("expected 2 underlying errors, got %d", len(joined.Unwrap()))
	}
	if !strings.Contains(err.Error(), string(FileAgents)) {
		t.Errorf("expected AGENTS.md in error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), string(FileSoul)) {
		t.Errorf("expected SOUL.md in error, got %q", err.Error())
	}
	if errors.Unwrap(err) != nil {
		t.Error("expected joined error to use multi-unwrap")
	}
}

func TestWatcherGetContent(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "# Agents")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
	})

	content, ok := w.GetContent(FileAgents)
	if !ok {
		t.Fatal("expected content")
	}
	if content != "# Agents" {
		t.Errorf("expected '# Agents', got %s", content)
	}

	_, ok = w.GetContent(FileSoul)
	if ok {
		t.Error("expected no content for missing file")
	}
}

func TestWatcherGetAll(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "agents")
	writeFile(t, dir, "SOUL.md", "soul")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents, FileSoul},
	})

	all := w.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 entries, got %d", len(all))
	}
}

func TestWatcherStartStop(t *testing.T) {
	dir := setupTestDir(t)

	w := NewWatcher(WatcherConfig{
		BaseDir:      dir,
		PollInterval: 100 * time.Millisecond,
		AutoLoad:     false,
	})

	if err := w.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	w.Stop()

	if err := w.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}

	w.Stop()
}

func TestWatcherStartTwice(t *testing.T) {
	dir := setupTestDir(t)

	w := NewWatcher(WatcherConfig{
		BaseDir:      dir,
		PollInterval: 100 * time.Millisecond,
	})

	w.Start()
	defer w.Stop()

	err := w.Start()
	if err == nil {
		t.Error("expected error on double start")
	}
}

func TestWatcherSameContentNoEvent(t *testing.T) {
	dir := setupTestDir(t)
	path := writeFile(t, dir, "AGENTS.md", "content")

	var changes []ChangeEvent
	var mu sync.Mutex

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: true,
		Files:    []FileType{FileAgents},
		OnChange: func(event ChangeEvent) {
			mu.Lock()
			changes = append(changes, event)
			mu.Unlock()
		},
	})

	// Touch file without changing content
	time.Sleep(10 * time.Millisecond)
	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	w.checkChanges()

	mu.Lock()
	if len(changes) > 0 {
		t.Errorf("expected no change event for same content, got %d", len(changes))
	}
	mu.Unlock()
}

func TestFileLoader(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "# Agents")
	writeFile(t, dir, "SOUL.md", "# Soul")

	loader := NewFileLoader(dir)

	entry, err := loader.Load(FileAgents)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if entry.Content != "# Agents" {
		t.Errorf("expected '# Agents', got %s", entry.Content)
	}

	// Should return cached entry
	entry2, err := loader.Load(FileAgents)
	if err != nil {
		t.Fatalf("load cached: %v", err)
	}
	if entry != entry2 {
		t.Error("expected same cached entry")
	}
}

func TestFileLoaderMissing(t *testing.T) {
	dir := setupTestDir(t)
	loader := NewFileLoader(dir)

	entry, err := loader.Load(FileAgents)
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if entry != nil {
		t.Error("expected nil entry for missing file")
	}
}

func TestFileLoaderLoadAll(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "agents")
	writeFile(t, dir, "SOUL.md", "soul")

	loader := NewFileLoader(dir)

	result, err := loader.LoadAll([]FileType{FileAgents, FileSoul, FileRules})
	if err != nil {
		t.Fatalf("load all: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestFileLoaderGet(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "agents")

	loader := NewFileLoader(dir)
	loader.Load(FileAgents)

	entry, ok := loader.Get(FileAgents)
	if !ok {
		t.Fatal("expected entry")
	}
	if entry.Content != "agents" {
		t.Errorf("expected 'agents', got %s", entry.Content)
	}

	_, ok = loader.Get(FileSoul)
	if ok {
		t.Error("expected no entry for unloaded file")
	}
}

func TestFileLoaderClear(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "AGENTS.md", "agents")

	loader := NewFileLoader(dir)
	loader.Load(FileAgents)

	loader.Clear()

	_, ok := loader.Get(FileAgents)
	if ok {
		t.Error("expected cleared entries")
	}
}

func TestWatcherCustomFile(t *testing.T) {
	dir := setupTestDir(t)
	writeFile(t, dir, "custom", "custom content")

	w := NewWatcher(WatcherConfig{
		BaseDir:  dir,
		AutoLoad: false,
	})

	w.loadFile(FileCustom)

	entry, ok := w.Get(FileCustom)
	if !ok {
		t.Fatal("expected custom file entry")
	}
	if entry.Content != "custom content" {
		t.Errorf("expected 'custom content', got %s", entry.Content)
	}
}
