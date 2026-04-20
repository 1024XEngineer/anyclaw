package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupDualMemory(t *testing.T) *DualMemory {
	t.Helper()
	dir := t.TempDir()
	dm, err := NewDualMemory(dir, "")
	if err != nil {
		t.Fatalf("failed to create dual memory: %v", err)
	}

	if err := dm.Init(); err != nil {
		t.Fatalf("failed to init dual memory: %v", err)
	}

	t.Cleanup(func() {
		dm.Close()
	})

	return dm
}

func TestDualMemoryAddAndGet(t *testing.T) {
	dm := setupDualMemory(t)

	entry := MemoryEntry{
		Type:     TypeFact,
		Content:  "The sky is blue",
		Metadata: map[string]string{"source": "observation"},
	}

	if err := dm.Add(entry); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	entries, err := dm.List()
	if err != nil {
		t.Fatalf("failed to list entries: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestDualMemorySearch(t *testing.T) {
	dm := setupDualMemory(t)

	dm.Add(MemoryEntry{Type: TypeFact, Content: "Go is a programming language"})
	dm.Add(MemoryEntry{Type: TypeFact, Content: "Python is also a programming language"})
	dm.Add(MemoryEntry{Type: TypeReflection, Content: "I like coding"})

	results, err := dm.Search("programming", 10)
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}
}

func TestDualMemoryConversationHistory(t *testing.T) {
	dm := setupDualMemory(t)
	svc := NewMemoryService(dm)

	dm.Add(MemoryEntry{Type: TypeConversation, Role: "user", Content: "Hello"})
	dm.Add(MemoryEntry{Type: TypeConversation, Role: "assistant", Content: "Hi there"})
	dm.Add(MemoryEntry{Type: TypeFact, Content: "Fact not conversation"})

	history, err := svc.GetConversationHistory(10)
	if err != nil {
		t.Fatalf("failed to get conversation history: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 conversation entries, got %d", len(history))
	}
}

func TestDualMemoryStats(t *testing.T) {
	dm := setupDualMemory(t)
	svc := NewMemoryService(dm)

	dm.Add(MemoryEntry{Type: TypeConversation, Content: "conv1"})
	dm.Add(MemoryEntry{Type: TypeFact, Content: "fact1"})
	dm.Add(MemoryEntry{Type: TypeReflection, Content: "reflect1"})

	stats, err := svc.GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats["total"] != 3 {
		t.Errorf("expected total 3, got %d", stats["total"])
	}
}

func TestDualMemorySyncFileToSQLite(t *testing.T) {
	dir := t.TempDir()
	dm, _ := NewDualMemory(dir, "")
	dm.Init()
	defer dm.Close()

	dm.SetSyncOnWrite(false)

	dm.file.Add(MemoryEntry{Type: TypeFact, Content: "File-only entry"})

	if err := dm.SyncFileToSQLite(); err != nil {
		t.Fatalf("failed to sync file to SQLite: %v", err)
	}

	entries, err := dm.sqlite.List()
	if err != nil {
		t.Fatalf("failed to list SQLite entries: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 SQLite entry after sync, got %d", len(entries))
	}
}

func TestDualMemorySyncSQLiteToFile(t *testing.T) {
	dir := t.TempDir()
	dm, _ := NewDualMemory(dir, "")
	dm.Init()
	defer dm.Close()

	dm.SetSyncOnWrite(false)

	dm.sqlite.Add(MemoryEntry{Type: TypeFact, Content: "SQLite-only entry"})

	if err := dm.SyncSQLiteToFile(); err != nil {
		t.Fatalf("failed to sync SQLite to file: %v", err)
	}

	entries, err := dm.file.List()
	if err != nil {
		t.Fatalf("failed to list file entries: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 file entry after sync, got %d", len(entries))
	}
}

func TestDualMemoryDelete(t *testing.T) {
	dm := setupDualMemory(t)

	entry := MemoryEntry{Type: TypeFact, Content: "To be deleted"}
	dm.Add(entry)

	entries, _ := dm.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry before delete")
	}

	if err := dm.Delete(entries[0].ID); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	entries, _ = dm.List()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after delete, got %d", len(entries))
	}
}

func TestDualMemoryAddKeepsFileAndSQLiteEntriesAligned(t *testing.T) {
	dm := setupDualMemory(t)

	if err := dm.Add(MemoryEntry{
		Type:    TypeFact,
		Content: "shared entry",
	}); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	sqliteEntries, err := dm.sqlite.List()
	if err != nil {
		t.Fatalf("failed to list sqlite entries: %v", err)
	}
	fileEntries, err := dm.file.List()
	if err != nil {
		t.Fatalf("failed to list file entries: %v", err)
	}

	if len(sqliteEntries) != 1 || len(fileEntries) != 1 {
		t.Fatalf("expected 1 mirrored entry in each backend, got sqlite=%d file=%d", len(sqliteEntries), len(fileEntries))
	}

	if sqliteEntries[0].ID != fileEntries[0].ID {
		t.Fatalf("expected mirrored entry IDs to match, got sqlite=%q file=%q", sqliteEntries[0].ID, fileEntries[0].ID)
	}
	if !sqliteEntries[0].Timestamp.Equal(fileEntries[0].Timestamp) {
		t.Fatalf("expected mirrored timestamps to match, got sqlite=%s file=%s", sqliteEntries[0].Timestamp.Format(time.RFC3339Nano), fileEntries[0].Timestamp.Format(time.RFC3339Nano))
	}
}

func TestDualMemoryDeleteRemovesFileMirror(t *testing.T) {
	dm := setupDualMemory(t)

	if err := dm.Add(MemoryEntry{
		Type:    TypeFact,
		Content: "delete me",
	}); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	fileEntries, err := dm.file.List()
	if err != nil {
		t.Fatalf("failed to list file entries before delete: %v", err)
	}
	if len(fileEntries) != 1 {
		t.Fatalf("expected 1 file entry before delete, got %d", len(fileEntries))
	}

	if err := dm.Delete(fileEntries[0].ID); err != nil {
		t.Fatalf("failed to delete entry: %v", err)
	}

	fileEntries, err = dm.file.List()
	if err != nil {
		t.Fatalf("failed to list file entries after delete: %v", err)
	}
	if len(fileEntries) != 0 {
		t.Fatalf("expected file mirror to be deleted, got %d remaining entries", len(fileEntries))
	}

	sqliteEntries, err := dm.sqlite.List()
	if err != nil {
		t.Fatalf("failed to list sqlite entries after delete: %v", err)
	}
	if len(sqliteEntries) != 0 {
		t.Fatalf("expected sqlite entry to be deleted, got %d remaining entries", len(sqliteEntries))
	}
}

func TestMemoryBackendFactory(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Backend: BackendFile,
		WorkDir: dir,
	}
	backend, err := NewMemoryBackend(cfg)
	if err != nil {
		t.Fatalf("failed to create file backend: %v", err)
	}
	if _, ok := backend.(*FileMemory); !ok {
		t.Error("expected FileMemory backend")
	}
	backend.Close()

	cfg = Config{
		Backend: BackendSQLite,
		WorkDir: dir,
	}
	backend, err = NewMemoryBackend(cfg)
	if err != nil {
		t.Fatalf("failed to create SQLite backend: %v", err)
	}
	if _, ok := backend.(*SQLiteMemory); !ok {
		t.Error("expected SQLiteMemory backend")
	}
	backend.Close()

	cfg = Config{
		Backend: BackendDual,
		WorkDir: dir,
	}
	backend, err = NewMemoryBackend(cfg)
	if err == nil {
		t.Fatal("expected generic factory to reject dual backend")
	}
}

func TestMigrateFileToSQLite(t *testing.T) {
	dir := t.TempDir()

	fileMem := NewFileMemory(dir)
	fileMem.Init()

	fileMem.Add(MemoryEntry{Type: TypeFact, Content: "Migrate me"})
	fileMem.Add(MemoryEntry{Type: TypeReflection, Content: "Another entry"})

	if err := MigrateFileToSQLite(dir, ""); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	sqliteMem, _ := NewSQLiteMemory(dir, "")
	sqliteMem.Init()
	defer sqliteMem.Close()

	entries, err := sqliteMem.List()
	if err != nil {
		t.Fatalf("failed to list after migration: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries after migration, got %d", len(entries))
	}
}

func TestSQLiteMemoryDirect(t *testing.T) {
	dir := t.TempDir()
	mem, err := NewSQLiteMemory(dir, ":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLite memory: %v", err)
	}

	if err := mem.InitWithDSN(":memory:"); err != nil {
		t.Fatalf("failed to init: %v", err)
	}
	defer mem.Close()

	if err := mem.Add(MemoryEntry{Type: TypeFact, Content: "Direct test"}); err != nil {
		t.Fatalf("failed to add: %v", err)
	}

	entries, err := mem.List()
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	results, err := mem.Search("Direct", 10)
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}

	stats, err := NewMemoryService(mem).GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats["total"] != 1 {
		t.Errorf("expected total 1, got %d", stats["total"])
	}
}

func TestSQLiteMemoryInitUsesConfiguredDSN(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom.db")

	mem, err := NewSQLiteMemory(dir, customPath)
	if err != nil {
		t.Fatalf("failed to create SQLite memory: %v", err)
	}

	if err := mem.Init(); err != nil {
		t.Fatalf("failed to init: %v", err)
	}
	defer mem.Close()

	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("expected custom sqlite db at %q: %v", customPath, err)
	}

	defaultPath := filepath.Join(dir, "memory.db")
	if _, err := os.Stat(defaultPath); err == nil {
		t.Fatalf("did not expect default sqlite db at %q", defaultPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat default sqlite db %q: %v", defaultPath, err)
	}
}
